package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kube-vip/kube-vip/pkg/k8s"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cmd"
)

type testConfig struct {
	ImagePath           string
	ControllerImagePath string
	Name                string

	serviceAddress string

	Services   bool
	forceClean bool
	cleanup    bool
	reload     bool
}

func main() {
	var t testConfig
	flag.StringVar(&t.Name, "name", "multi-cluster", "")

	flag.StringVar(&t.ImagePath, "imagepath", "plndr/kube-vip:action", "")
	flag.StringVar(&t.ControllerImagePath, "controllerimagepath", "controller:latest", "")

	flag.StringVar(&t.serviceAddress, "svcAddress", "", "")

	flag.BoolVar(&t.cleanup, "cleanup", false, "automatically remove the cluster at the end")
	flag.BoolVar(&t.forceClean, "forceClean", false, "automatically remove the cluster at the end")

	flag.Parse()

	log.Infof("🔬 beginning e2e tests, image: [%s]", t.ImagePath)

	if t.forceClean {
		provider = cluster.NewProvider(cluster.ProviderWithLogger(cmd.NewLogger()), cluster.ProviderWithDocker())
		err := t.deleteKind()
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	err := t.createKind()
	if t.cleanup {
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			err := t.deleteKind()
			if err != nil {
				log.Fatal(err)
			}
		}()
	} else {
		if err != nil {
			log.Warn(err)
		}
	}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	homeConfigPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	clientset, err := k8s.NewClientset(homeConfigPath, false, "")
	if err != nil {
		log.Fatalf("could not create k8s clientset from external file: %q: %v", homeConfigPath, err)
	}
	log.Debugf("Using external Kubernetes configuration from file [%s]", homeConfigPath)

	// Deplopy the daemonset for kube-vip
	deploy := deployment{}
	err = deploy.createKVDs(ctx, clientset, t.ImagePath)
	if err != nil {
		log.Error(err)
	}
	s := service{
		name:      "multi-cluster",
		namespace: "kube-vip-multi-cluster-system",
		port:      9990,
		address:   t.serviceAddress,
	}
	a, b, err := s.createService(ctx, clientset)
	if err != nil {
		fmt.Printf("%s %s %s", a, b, err)
	}
	//t.startServiceTest(ctx, clientset)
	log.Infof("🎉 i recommend alias kubectl=\"kubectl --context kind-%s\"", t.Name)
}
