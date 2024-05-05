/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kubevipiov1alpha1 "github.com/kube-vip/kube-vip-multi-cluster/api/v1alpha1"
	"github.com/kube-vip/kube-vip-multi-cluster/pkg/payload"
	"github.com/sirupsen/logrus"
	"github.com/thebsdbox/navy/pkg/navy"
)

const finalizer = "kube-vip.io/finalizer"

// MultiClusterReconciler reconciles a MultiCluster object
type MultiClusterReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Config         *rest.Config
	CaptainTracker map[int]*navy.Captain
	dynClient      *dynamic.DynamicClient
	discClient     *discovery.DiscoveryClient
}

//+kubebuilder:rbac:groups=kube-vip.io,resources=multiclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kube-vip.io,resources=multiclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kube-vip.io,resources=multiclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MultiCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *MultiClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//log := log.FromContext(ctx)
	log := logrus.New()
	logrus.SetLevel(logrus.DebugLevel)

	var err error

	config := &kubevipiov1alpha1.MultiCluster{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		if errors.IsNotFound(err) {
			// object not found, could have been deleted after
			// reconcile request, hence don't requeue

			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Directions object")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// your logic here
	log.Infof("reconciling multicluster [%s]", config.Name)

	// Create clients for the payload
	r.dynClient, err = dynamic.NewForConfig(r.Config)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.discClient, err = discovery.NewDiscoveryClientForConfig(r.Config)
	if err != nil {
		return ctrl.Result{}, err
	}

	// this is already running
	if config.Status.Ready {
		n := r.CaptainTracker[config.Spec.Port]
		if n != nil {
			if !config.ObjectMeta.DeletionTimestamp.IsZero() {
				log.Infof("[DELETING] %s", config.Name)
				err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
					err = r.Get(ctx, req.NamespacedName, config, &client.GetOptions{})
					if err != nil {
						return err
					}
					controllerutil.RemoveFinalizer(config, finalizer)
					err = r.Update(context.TODO(), config)
					if err != nil {
						return err
					}
					return nil
				})
				n.LeaveFleet()
				n.Resign()
				// remove the entry
				delete(r.CaptainTracker, config.Spec.Port)
			}
		} else {
			log.Warnf("this cluster is ready [%s], but has no running leaderElection", config.Name)
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {

				err = r.Get(ctx, req.NamespacedName, config, &client.GetOptions{})
				if err != nil {
					return fmt.Errorf("getting updated config [%s]", err)
				}
				// Update the status of the object
				config.Status.Ready = false
				err = r.Status().Update(context.TODO(), config, &client.SubResourceUpdateOptions{})
				if err != nil {
					return fmt.Errorf("[1] updating status [%s]", err)
				}
				return nil
			})
		}
		return ctrl.Result{}, nil
	}

	// Create the leaderElection entity
	var bindaddr, extaddr string
	if config.Spec.Address == "" {
		bindaddr = fmt.Sprintf("0.0.0.0:%d", config.Spec.Port)
	} else {
		bindaddr = fmt.Sprintf("%s:%d", config.Spec.Address, config.Spec.Port)
	}
	if config.Spec.LoadBalancer != "" {
		extaddr = fmt.Sprintf("%s:%d", config.Spec.LoadBalancer, config.Spec.Port)
	}

	c := navy.NewCaptain(config.Spec.Rank, bindaddr, extaddr, "tcp4", config.Spec.Callsign, config.Spec.Fleet, config.Spec.Ready, false, nil)

	// Wrap the functions
	promotionFN := func(exit chan interface{}) {
		if c.GetLeaderPayload() != config.Spec.Payload {
			log.Warn("Different Payload")
		}
		err := r.promoted(config, log)
		if err != nil {
			log.Errorf("[Promotion] %s", err)
		}
		log.Info("[LEADER]")
		close(exit)
	}

	// _ := func(exit chan interface{}) {
	// 	log.Info("[Start demotion]")
	// 	if r == nil {
	// 		log.Info("uhho")
	// 	}
	// 	err := r.demoted(config, c.LeaderAddress(), log)
	// 	if err != nil {
	// 		log.Errorf("[Demotion] %s", err)
	// 	}
	// 	log.Info("[FOLLOWER]")
	// 	close(exit)
	// }

	// Map the functions to the callbacks
	c.OnPromotion(promotionFN)
	c.OnDemotion(func(exit chan interface{}) {
		log.Info("[Start demotion]")
		objs, err := payload.Decode([]byte(config.Spec.Payload))
		if err != nil {
			log.Errorf("[Demotion] %s", err)
		}
		for _, obj := range objs {
			log.Infof("deleting [%s]", obj.GetName())
			mapping, err := payload.GetRestMapping(r.discClient, obj.GetObjectKind().GroupVersionKind())
			if err != nil {
				log.Errorf("[Demotion] %s", err)
			}
			var dri dynamic.ResourceInterface

			if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
				if obj.GetNamespace() == "" {
					obj.SetNamespace("default")
				}
				dri = r.dynClient.Resource(mapping.Resource).Namespace(obj.GetNamespace())
			} else {
				dri = r.dynClient.Resource(mapping.Resource)
			}
			err = dri.Delete(context.TODO(), obj.GetName(), v1.DeleteOptions{})
			if err != nil {
				log.Errorf("unable to remove [%s]. [%s]", obj.GetName(), err)
			}
		}
		log.Info("[FOLLOWER]")
		close(exit)
	})

	// Various updates are wrapped in the retry on conflict

	// Update the object status
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err = r.Get(ctx, req.NamespacedName, config, &client.GetOptions{})
		if err != nil {
			return fmt.Errorf("getting updated config [%s]", err)
		}
		// Update the status of the object
		config.Status.Port = config.Spec.Port
		config.Status.Ready = true
		err = r.Status().Update(context.TODO(), config, &client.SubResourceUpdateOptions{})
		if err != nil {
			return fmt.Errorf("[2] updating status [%s]", err)
		}
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	// Start the listener!
	err = c.Listen()
	if err != nil {
		log.Error(err)
	}
	readyWatcher := make(chan interface{})
	go c.DiscoverResponse(readyWatcher)
	if !config.Spec.Ready {

		log.Info("Attepting to discover the fleet, with a backoff")
		err = c.DiscoverWithBackoff(navy.Backoff{MaxRetries: 5, Delay: time.Second})
		if err != nil {
			c.LeaveFleet()
			close(readyWatcher)
			c.Resign()
			//c.Resign()
			return ctrl.Result{}, err
		}

		log.Debug("Waiting to become ready")
		// hang about here until we're ready
		<-readyWatcher
		//n.DiscoverResponse()
	}

	// Start the leaderElection
	go c.Run(nil)

	r.CaptainTracker[config.Spec.Port] = c
	log.Infof("cluster [%s] rank [%d] on [%s] is [%t]", config.Name, config.Spec.Rank, extaddr, config.Status.Ready)

	// Add the finalizer to the object
	if !controllerutil.ContainsFinalizer(config, finalizer) {
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err = r.Get(ctx, req.NamespacedName, config, &client.GetOptions{})
			if err != nil {
				return fmt.Errorf("getting updated config [%s]", err)
			}
			controllerutil.AddFinalizer(config, finalizer)
			err = r.Update(context.TODO(), config, &client.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("adding finalizer [%s]", err)
			}
			return nil
		})
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *MultiClusterReconciler) promoted(config *kubevipiov1alpha1.MultiCluster, log *logrus.Logger) error {
	err := r.applyPayload(config, log)
	if err != nil {
		return err
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {

		config.Status.Leading = true
		config.Status.LeaderAddress = config.Spec.Address
		t := types.NamespacedName{
			Namespace: config.Namespace,
			Name:      config.Name,
		}
		if err := r.Get(context.TODO(), t, config, &client.GetOptions{}); err != nil {
			return err
		}
		err = r.Status().Update(context.TODO(), config, &client.SubResourceUpdateOptions{})
		if err != nil {
			return err
		}
		return nil
	})
}

func (r *MultiClusterReconciler) demoted(config *kubevipiov1alpha1.MultiCluster, leader string, log *logrus.Logger) error {
	log.Info("demotion")
	err := r.deletePayload(config, log)
	if err != nil {
		return err
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {

		config.Status.Leading = false
		config.Status.LeaderAddress = leader
		t := types.NamespacedName{
			Namespace: config.Namespace,
			Name:      config.Name,
		}
		if err := r.Get(context.TODO(), t, config, &client.GetOptions{}); err != nil {
			return err
		}
		err = r.Status().Update(context.TODO(), config, &client.SubResourceUpdateOptions{})
		if err != nil {
			return err
		}
		return nil
	})
}

func (r *MultiClusterReconciler) deletePayload(config *kubevipiov1alpha1.MultiCluster, log *logrus.Logger) error {
	objs, err := payload.Decode([]byte(config.Spec.Payload))
	if err != nil {
		return err
	}
	for _, obj := range objs {
		log.Infof("deleting [%s]", obj.GetName())
		mapping, err := payload.GetRestMapping(r.discClient, obj.GetObjectKind().GroupVersionKind())
		if err != nil {
			return err
		}
		var dri dynamic.ResourceInterface

		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			if obj.GetNamespace() == "" {
				obj.SetNamespace("default")
			}
			dri = r.dynClient.Resource(mapping.Resource).Namespace(obj.GetNamespace())
		} else {
			dri = r.dynClient.Resource(mapping.Resource)
		}
		err = dri.Delete(context.TODO(), obj.GetName(), v1.DeleteOptions{})
		if err != nil {
			log.Errorf("unable to remove [%s]. [%s]", obj.GetName(), err)
		}
	}
	return nil
}

func (r *MultiClusterReconciler) applyPayload(config *kubevipiov1alpha1.MultiCluster, log *logrus.Logger) error {
	objs, err := payload.Decode([]byte(config.Spec.Payload))
	if err != nil {
		return err
	}
	for _, obj := range objs {
		log.Infof("creating [%s]", obj.GetName())
		mapping, err := payload.GetRestMapping(r.discClient, obj.GetObjectKind().GroupVersionKind())
		if err != nil {
			return err
		}
		var dri dynamic.ResourceInterface

		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			if obj.GetNamespace() == "" {
				obj.SetNamespace("default")
			}
			dri = r.dynClient.Resource(mapping.Resource).Namespace(obj.GetNamespace())
		} else {
			dri = r.dynClient.Resource(mapping.Resource)
		}
		_, err = dri.Get(context.TODO(), obj.GetName(), v1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return fmt.Errorf("retrieving current configuration of:\n%s\nfrom server for:%v", obj.GetName(), err)
			}
			_, err = dri.Create(context.TODO(), &obj, v1.CreateOptions{})
			if err != nil {
				log.Errorf("unable to create object [%s]", obj.GetName())
			}
		}

	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MultiClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubevipiov1alpha1.MultiCluster{}).
		Complete(r)
}
