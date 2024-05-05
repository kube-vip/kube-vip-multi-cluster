package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	kindconfigv1alpha4 "sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cmd"
	load "sigs.k8s.io/kind/pkg/cmd/kind/load/docker-image"
)

var provider *cluster.Provider

type kubevipManifestValues struct {
	ControlPlaneVIP string
	ImagePath       string
}

type nodeAddresses struct {
	node      string
	addresses []string
}

func (config *testConfig) createKind() error {

	clusterConfig := kindconfigv1alpha4.Cluster{
		Networking: kindconfigv1alpha4.Networking{
			IPFamily: kindconfigv1alpha4.IPv4Family,
		},
		Nodes: []kindconfigv1alpha4.Node{
			{
				Role: kindconfigv1alpha4.ControlPlaneRole,
			},
		},
	}

	// Add one additional worker nodes
	clusterConfig.Nodes = append(clusterConfig.Nodes, kindconfigv1alpha4.Node{Role: kindconfigv1alpha4.WorkerRole})

	provider = cluster.NewProvider(cluster.ProviderWithLogger(cmd.NewLogger()), cluster.ProviderWithDocker())
	clusters, err := provider.List()
	if err != nil {
		return err
	}
	found := false
	for x := range clusters {
		if clusters[x] == config.Name {
			log.Infof("Cluster already exists")
			found = true
		}
	}
	if !found {
		err := provider.Create(config.Name, cluster.CreateWithV1Alpha4Config(&clusterConfig))
		if err != nil {
			return err

		}
		loadImageCmd := load.NewCommand(cmd.NewLogger(), cmd.StandardIOStreams())
		loadImageCmd.SetArgs([]string{"--name", config.Name, config.ImagePath})
		err = loadImageCmd.Execute()
		if err != nil {
			return err
		}
		loadImageCmd.SetArgs([]string{"--name", config.Name, config.ControllerImagePath})
		err = loadImageCmd.Execute()
		if err != nil {
			return err
		}
		nodes, err := provider.ListNodes(config.Name)
		if err != nil {
			return err
		}

		// HMMM, if we want to run workloads on the control planes (todo)
		for x := range nodes {
			cmd := exec.Command("kubectl", "taint", "nodes", nodes[x].String(), "node-role.kubernetes.io/control-plane:NoSchedule-") //nolint:all
			_, _ = cmd.CombinedOutput()
		}

		globalRange := "172.18.100.10-172.18.100.30"

		cmd := exec.Command("kubectl", "create", "configmap", "--namespace", "kube-system", "kubevip", "--from-literal", "range-global="+globalRange)
		if _, err := cmd.CombinedOutput(); err != nil {
			return err
		}
		cmd = exec.Command("kubectl", "create", "-f", "https://raw.githubusercontent.com/kube-vip/kube-vip-cloud-provider/main/manifest/kube-vip-cloud-controller.yaml")
		if _, err := cmd.CombinedOutput(); err != nil {
			return err
		}
		cmd = exec.Command("kubectl", "create", "-f", "https://kube-vip.io/manifests/rbac.yaml")
		if _, err := cmd.CombinedOutput(); err != nil {
			return err
		}

		// // Create the namespace
		// cmd = exec.Command("kubectl", "create", "namespace", "system")
		// if _, err := cmd.CombinedOutput(); err != nil {
		// 	return err
		// }

		// Apply the RBAC
		// for _, file := range []string{"service_account.yaml",
		// 	"role.yaml",
		// 	"role_binding.yaml",
		// 	"leader_election_role.yaml",
		// 	"leader_election_role_binding.yaml"} {
		// 	cmd = exec.Command("kubectl", "create", "-f", "./config/rbac/"+file)
		// 	if _, err := cmd.CombinedOutput(); err != nil {
		// 		return err
		// 	}
		// }

		// Deploy the controller !
		log.Info("🤖 deploying Controller")
		cmd = exec.Command("kubectl", "delete", "-f", "./dist/install.yaml")
		cmd.CombinedOutput() // Ignore if this is succesful or not
		// if b, err := cmd.CombinedOutput(); err != nil {
		// 	return fmt.Errorf("%s, [%s]", b, err)
		// }
		cmd = exec.Command("kubectl", "apply", "-f", "./dist/install.yaml")
		if b, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s, [%s]", b, err)
		}
		log.Infof("💤 sleeping for a few seconds to let controllers start")
		time.Sleep(time.Second * 5)
	}
	return nil
}

func (config *testConfig) deleteKind() error {
	log.Info("🧽 deleting Kind cluster")
	return provider.Delete(config.Name, "")
}

func getAddressesOnNodes() ([]nodeAddresses, error) {
	nodesConfig := []nodeAddresses{}
	nodes, err := provider.ListNodes("services")
	if err != nil {
		return nodesConfig, err
	}
	for x := range nodes {
		var b bytes.Buffer

		exec := nodes[x].Command("hostname", "--all-ip-addresses")
		exec.SetStderr(&b)
		exec.SetStdin(&b)
		exec.SetStdout(&b)
		err = exec.Run()
		if err != nil {
			return nodesConfig, err
		}
		nodesConfig = append(nodesConfig, nodeAddresses{
			node:      nodes[x].String(),
			addresses: strings.Split(b.String(), " "),
		})
	}
	return nodesConfig, nil
}

func checkNodesForDuplicateAddresses(nodes []nodeAddresses, address string) error {
	var foundOnNode []string
	// Iterate over all nodes to find addresses, where there is an address match add to array
	for x := range nodes {
		for y := range nodes[x].addresses {
			if nodes[x].addresses[y] == address {
				foundOnNode = append(foundOnNode, nodes[x].node)
			}
		}
	}
	// If one address is on multiple nodes, then something has gone wrong
	if len(foundOnNode) > 1 {
		return fmt.Errorf("‼️ multiple nodes [%s] have address [%s]", strings.Join(foundOnNode, " "), address)
	}
	return nil
}
