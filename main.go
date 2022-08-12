package main

import (
	"context"
	"io/ioutil"
	"log"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Config struct {
	Version   string            `yaml:"version"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels"`
}

func main() {
	c := readConfig()
	log.Printf("starting pm-tagger version: %s\n", c.Version)
	saToken, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	clientset, err := kubernetes.NewForConfig(saToken)
	if err != nil {
		panic(err.Error())
	}

	lbls := labels.Set(c.Labels).String()

	for {
		podList, err := clientset.CoreV1().Pods(c.Namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: lbls,
		})
		if errors.IsNotFound(err) {
			log.Printf("No gateway pods found\n")
		} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
			log.Printf("Error getting gateway pod list %v\n", statusError.ErrStatus.Message)
		} else if err != nil {
			panic(err.Error())
		} else {
			var managementPod bool
			for _, pod := range podList.Items {
				for k, v := range pod.Labels {
					if k == "management-access" && v == "leader" {
						managementPod = true
						for _, s := range pod.Status.ContainerStatuses {
							if s.Name == "gateway" && s.Ready {
								log.Printf("management pod %s is ready for connections", pod.Name)
							}
							if !s.Ready {
								log.Printf("management pod %s is not ready for connections", pod.Name)
							}
						}
					}
				}
			}
			if !managementPod && len(podList.Items) > 0 {
				log.Println("tagging a new management pod")
				patch := []byte(`{"metadata":{"labels":{"management-access": "leader"}}}`)
				_, err = clientset.CoreV1().Pods(c.Namespace).Patch(context.TODO(), podList.Items[0].Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
				if err != nil {
					panic(err.Error())
				}
				log.Printf("the new management pod is %s", podList.Items[0].Name)
			}
		}

		time.Sleep(10 * time.Second)
	}
}

func readConfig() Config {
	config := Config{}
	confBytes, err := ioutil.ReadFile("./config.yaml")

	if err != nil {
		panic(err.Error())
	}

	err = yaml.Unmarshal(confBytes, &config)
	if err != nil {
		panic(err.Error())
	}

	return config
}
