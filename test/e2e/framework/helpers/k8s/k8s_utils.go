/*
 Licensed to the Apache Software Foundation (ASF) under one
 or more contributor license agreements.  See the NOTICE file
 distributed with this work for additional information
 regarding copyright ownership.  The ASF licenses this file
 to you under the Apache License, Version 2.0 (the
 "License"); you may not use this file except in compliance
 with the License.  You may obtain a copy of the License at
     http://www.apache.org/licenses/LICENSE-2.0
 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package k8s

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/apache/incubator-yunikorn-k8shim/test/e2e/framework/helpers/common"

	"k8s.io/apimachinery/pkg/util/wait"

	v1 "k8s.io/api/core/v1"
	authv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/apache/incubator-yunikorn-k8shim/test/e2e/framework/configmanager"
)

type KubeCtl struct {
	clientSet      *kubernetes.Clientset
	kubeConfigPath string
	kubeConfig     *rest.Config
}

// findKubeConfig finds path from env:KUBECONFIG or ~/.kube/config
func (k *KubeCtl) findKubeConfig() error {
	env := os.Getenv("KUBECONFIG")
	if env != "" {
		k.kubeConfigPath = env
		return nil
	}
	var err error
	k.kubeConfigPath, err = expand(configmanager.YuniKornTestConfig.KubeConfig)
	return err
}

func expand(path string) (string, error) {
	if len(path) == 0 {
		return path, nil
	}

	if path[0] != '~' {
		return path, nil
	}

	if len(path) > 1 && path[1] != '/' && path[1] != '\\' {
		return "", errors.New("cannot expand user-specific home dir")
	}

	dir, err1 := os.UserHomeDir()
	if err1 != nil {
		return "", err1
	}

	return filepath.Join(dir, path[1:]), nil
}

func (k *KubeCtl) SetClient() error {
	var err error
	if k.findKubeConfig() != nil {
		return err
	}
	k.kubeConfig, err = clientcmd.BuildConfigFromFlags("", k.kubeConfigPath)
	if err != nil {
		return err
	}
	k.clientSet, err = kubernetes.NewForConfig(k.kubeConfig)
	return err
}

func (k *KubeCtl) GetKubeConfig() (*rest.Config, error) {
	if k.kubeConfig != nil {
		return k.kubeConfig, nil
	}
	return nil, errors.New("kubeconfig is nil")
}
func (k *KubeCtl) GetPods(namespace string) (*v1.PodList, error) {
	return k.clientSet.CoreV1().Pods(namespace).List(metav1.ListOptions{})
}

func (k *KubeCtl) GetPodNamesFromNS(namespace string) ([]string, error) {
	var s []string
	Pods, err := k.GetPods(namespace)
	if err != nil {
		return nil, err
	}
	for _, each := range Pods.Items {
		s = append(s, each.Name)
	}
	return s, nil
}

func (k *KubeCtl) GetService(serviceName string, namespace string) (*v1.Service, error) {
	return k.clientSet.CoreV1().Services(namespace).Get(serviceName, metav1.GetOptions{})
}

// Func to create a namespace provided a name
func (k *KubeCtl) CreateNamespace(namespace string, annotations map[string]string) (*v1.Namespace, error) {
	return k.clientSet.CoreV1().Namespaces().Create(&v1.Namespace{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        namespace,
			Labels:      map[string]string{"Name": namespace},
			Annotations: annotations,
		},
		Spec:   v1.NamespaceSpec{},
		Status: v1.NamespaceStatus{},
	})
}

func (k *KubeCtl) DeleteNamespace(namespace string) error {
	return k.clientSet.CoreV1().Namespaces().Delete(namespace, &metav1.DeleteOptions{})
}

func (k *KubeCtl) TearDownNamespace(namespace string) error {
	// Delete all pods
	var pods, err = k.GetPodNamesFromNS(namespace)
	if err != nil {
		return err
	}
	for _, each := range pods {
		err = k.DeletePod(each, namespace)
		if err != nil {
			return err
		}
	}

	// Delete namespace
	return k.clientSet.CoreV1().Namespaces().Delete(namespace, &metav1.DeleteOptions{})
}

func GetConfigMapObj(yamlPath string) (*v1.ConfigMap, error) {
	c, err := common.Yaml2Obj(yamlPath)
	if err != nil {
		return nil, err
	}
	return c.(*v1.ConfigMap), err
}

func (k *KubeCtl) CreateConfigMap(cMap *v1.ConfigMap, namespace string) (*v1.ConfigMap, error) {
	return k.clientSet.CoreV1().ConfigMaps(namespace).Create(cMap)
}

func (k *KubeCtl) GetConfigMap(name string, namespace string) (*v1.ConfigMap, error) {
	return k.clientSet.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
}

func (k *KubeCtl) UpdateConfigMap(cMap *v1.ConfigMap, namespace string) (*v1.ConfigMap, error) {
	return k.clientSet.CoreV1().ConfigMaps(namespace).Update(cMap)
}

func (k *KubeCtl) DeleteConfigMap(cName string, namespace string) error {
	return k.clientSet.CoreV1().ConfigMaps(namespace).Delete(cName, &metav1.DeleteOptions{})
}

func GetPodObj(yamlPath string) (*v1.Pod, error) {
	o, err := common.Yaml2Obj(yamlPath)
	if err != nil {
		return nil, err
	}
	return o.(*v1.Pod), err
}

func (k *KubeCtl) CreatePod(pod *v1.Pod, namespace string) (*v1.Pod, error) {
	return k.clientSet.CoreV1().Pods(namespace).Create(pod)
}

func (k *KubeCtl) DeletePod(podName string, namespace string) error {
	err := k.clientSet.CoreV1().Pods(namespace).Delete(podName, &metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = k.WaitForPodTerminated(namespace, podName, 60*time.Second)
	return err
}

// return a condition function that indicates whether the given pod is
// currently in desired state
func (k *KubeCtl) isPodInDesiredState(podName string, namespace string, state v1.PodPhase) wait.ConditionFunc {
	return func() (bool, error) {
		fmt.Printf(".") // progress bar!

		pod, err := k.clientSet.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		switch pod.Status.Phase {
		case state:
			return true, nil
		case v1.PodUnknown:
			return false, fmt.Errorf("pod is in unknown state")
		}
		return false, nil
	}
}

// Return a condition function that indicates if the pod is NOT in the given namespace
func (k *KubeCtl) isPodNotInNS(podName string, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		fmt.Printf(".") // progress bar!

		podNames, err := k.GetPodNamesFromNS(namespace)
		if err != nil {
			return false, err
		}
		for _, name := range podNames {
			if podName == name {
				return false, nil
			}
		}
		return true, nil
	}
}

func (k *KubeCtl) WaitForPodTerminated(namespace string, podName string, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout*time.Second, k.isPodNotInNS(podName, namespace))
}

// Poll up to timeout seconds for pod to enter running state.
// Returns an error if the pod never enters the running state.
func (k *KubeCtl) WaitForPodRunning(namespace string, podName string, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, k.isPodInDesiredState(podName, namespace, v1.PodRunning))
}

func (k *KubeCtl) WaitForPodPending(namespace string, podName string, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, k.isPodInDesiredState(podName, namespace, v1.PodPending))
}

func (k *KubeCtl) WaitForPodSucceeded(namespace string, podName string, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, k.isPodInDesiredState(podName, namespace, v1.PodSucceeded))
}

func (k *KubeCtl) WaitForPodFailed(namespace string, podName string, timeout time.Duration) error {
	return wait.PollImmediate(time.Second, timeout, k.isPodInDesiredState(podName, namespace, v1.PodFailed))
}

// Returns the list of currently scheduled or running pods in `namespace` with the given selector
func (k *KubeCtl) ListPods(namespace string, selector string) (*v1.PodList, error) {
	listOptions := metav1.ListOptions{LabelSelector: selector}
	podList, err := k.clientSet.CoreV1().Pods(namespace).List(listOptions)

	if err != nil {
		return nil, err
	}
	return podList, nil
}

// Wait up to timeout seconds for all pods in 'namespace' with given 'selector' to enter running state.
// Returns an error if no pods are found or not all discovered pods enter running state.
func (k *KubeCtl) WaitForPodBySelectorRunning(namespace string, selector string, timeout int) error {
	podList, err := k.ListPods(namespace, selector)
	if err != nil {
		return err
	}
	if len(podList.Items) == 0 {
		return fmt.Errorf("no pods in %s with selector %s", namespace, selector)
	}

	for _, pod := range podList.Items {
		if err := k.WaitForPodRunning(namespace, pod.Name, time.Duration(timeout)*time.Second); err != nil {
			return err
		}
	}
	return nil
}

func (k *KubeCtl) CreateSecret(secret *v1.Secret, namespace string) (*v1.Secret, error) {
	return k.clientSet.CoreV1().Secrets(namespace).Create(secret)
}

func GetSecretObj(yamlPath string) (*v1.Secret, error) {
	o, err := common.Yaml2Obj(yamlPath)
	if err != nil {
		return nil, err
	}
	return o.(*v1.Secret), err
}

func (k *KubeCtl) CreateServiceAccount(accountName string, namespace string) (*v1.ServiceAccount, error) {
	return k.clientSet.CoreV1().ServiceAccounts(namespace).Create(&v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: accountName},
	})
}

func (k *KubeCtl) DeleteServiceAccount(accountName string, namespace string) error {
	return k.clientSet.CoreV1().ServiceAccounts(namespace).Delete(accountName, &metav1.DeleteOptions{})
}

func (k *KubeCtl) CreateClusterRoleBinding(
	roleName string,
	role string,
	namespace string,
	serviceAccount string) (*authv1.ClusterRoleBinding, error) {
	return k.clientSet.RbacV1().ClusterRoleBindings().Create(&authv1.ClusterRoleBinding{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{Name: roleName},
		Subjects: []authv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccount,
				Namespace: namespace,
			},
		},
		RoleRef: authv1.RoleRef{Name: role, Kind: "ClusterRole"},
	})
}

func (k *KubeCtl) DeleteClusterRoleBindings(roleName string) error {
	return k.clientSet.RbacV1().ClusterRoleBindings().Delete(roleName, &metav1.DeleteOptions{})
}

func (k *KubeCtl) GetConfigMaps(namespace string, cMapName string) (*v1.ConfigMap, error) {
	return k.clientSet.CoreV1().ConfigMaps(namespace).Get(cMapName, metav1.GetOptions{})
}

func (k *KubeCtl) UpdateConfigMaps(namespace string, cMap *v1.ConfigMap) (*v1.ConfigMap, error) {
	return k.clientSet.CoreV1().ConfigMaps(namespace).Update(cMap)
}

func (k *KubeCtl) DeleteConfigMaps(namespace string, cMapName string) error {
	return k.clientSet.CoreV1().ConfigMaps(namespace).Delete(cMapName, &metav1.DeleteOptions{})
}

func (k *KubeCtl) CreateConfigMaps(namespace string, cMap *v1.ConfigMap) (*v1.ConfigMap, error) {
	return k.clientSet.CoreV1().ConfigMaps(namespace).Create(cMap)
}
