// Package pods provides implementation of Pod resources for Kubernetes
//
// Deprecated: Use the resources package instead.
package pods

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	k8sTypes "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// ValidPod status
const (
	Running   string = "Running"
	Succeeded string = "Succeeded"
)

// New creates a new instance backed by the provided client
//
// Deprecated: No longer used.
func New(ctx context.Context, client kubernetes.Interface, config *rest.Config, metaOptions metav1.ListOptions) *Pods {
	return &Pods{
		client,
		config,
		metaOptions,
		ctx,
	}
}

// ExecOptions describe the command to be executed and the target container
//
// Deprecated: No longer used.
type ExecOptions struct {
	Namespace string   // namespace where the pod is running
	Pod       string   // name of the Pod to execute the command in
	Container string   // name of the container to execute the command in
	Command   []string // command to be executed with its parameters
	Stdin     []byte   // stdin to be supplied to the command
}

// ExecResult contains the output obtained from the execution of a command
//
// Deprecated: No longer used.
type ExecResult struct {
	Stdout []byte
	Stderr []byte
}

// ContainerOptions describes a container to be started in a pod
//
// Deprecated: No longer used.
type ContainerOptions struct {
	Name         string // name of the container
	Image        string // image to be attached
	PullPolicy   k8sTypes.PullPolicy
	Command      []string // command to be executed by the container
	Capabilities []string // capabilities to be added to the container's security context
}

// EphemeralContainerOptions describes the options for creating an ephemeral container in a pod
//
// Deprecated: No longer used.
type EphemeralContainerOptions struct {
	ContainerOptions
	Wait string
}

// Pods provides API for manipulating Pod resources within a Kubernetes cluster
//
// Deprecated: No longer used in favor of generic resources.
type Pods struct {
	client      kubernetes.Interface
	config      *rest.Config
	metaOptions metav1.ListOptions
	ctx         context.Context
}

// PodOptions describe a Pod to be executed
//
// Deprecated: No longer used.
type PodOptions struct {
	Namespace     string // namespace where the pod will be executed
	Name          string // name of the pod
	Image         string // image to be executed by the pod's container
	PullPolicy    k8sTypes.PullPolicy
	Command       []string               // command to be executed by the pod's container and its arguments
	RestartPolicy k8sTypes.RestartPolicy // policy for restarting containers in the pod [Always|OnFailure|Never]
	Wait          string                 // timeout for waiting until the pod is running
}

// podConditionChecker defines a function that checks if a pod satisfies a condition
//
// Deprecated: No longer used.
type podConditionChecker func(*k8sTypes.Pod) (bool, error)

// List returns a collection of Pods available within the namespace
//
// Deprecated: Use resources.List instead.
func (obj *Pods) List(namespace string) ([]k8sTypes.Pod, error) {
	pods, err := obj.client.CoreV1().Pods(namespace).List(obj.ctx, obj.metaOptions)
	if err != nil {
		return []k8sTypes.Pod{}, err
	}
	return pods.Items, nil
}

// Delete removes the named Pod from the namespace
//
// Deprecated: Use resources.Delete instead.
func (obj *Pods) Delete(name, namespace string, opts metav1.DeleteOptions) error {
	return obj.client.CoreV1().Pods(namespace).Delete(obj.ctx, name, opts)
}

// Kill removes the named Pod from the namespace
//
// Deprecated: Use resources.Delete instead.
func (obj *Pods) Kill(name, namespace string, opts metav1.DeleteOptions) error {
	return obj.Delete(name, namespace, opts)
}

// Get returns the named Pods instance within the namespace if available
//
// Deprecated: Use resources.Get instead.
func (obj *Pods) Get(name, namespace string) (k8sTypes.Pod, error) {
	pods, err := obj.List(namespace)
	if err != nil {
		return k8sTypes.Pod{}, err
	}
	for _, pod := range pods {
		if pod.Name == name {
			return pod, nil
		}
	}
	return k8sTypes.Pod{}, errors.New(name + " pod not found")
}

// IsTerminating returns if the state of the named pod is currently terminating
//
// Deprecated: No longer used.
func (obj *Pods) IsTerminating(name, namespace string) (bool, error) {
	pod, err := obj.Get(name, namespace)
	if err != nil {
		return false, err
	}
	return (pod.ObjectMeta.DeletionTimestamp != nil), nil
}

// Create runs a pod specified by the options
//
// Deprecated: Use resources.Create instead.
func (obj *Pods) Create(options PodOptions) (k8sTypes.Pod, error) {
	container := k8sTypes.Container{
		Name:            options.Name,
		Image:           options.Image,
		ImagePullPolicy: options.PullPolicy,
		Command:         options.Command,
	}

	containers := []k8sTypes.Container{
		container,
	}

	var restartPolicy k8sTypes.RestartPolicy = "Never"

	if options.RestartPolicy != "" {
		restartPolicy = options.RestartPolicy
	}

	newPod := k8sTypes.Pod{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{Name: options.Name},
		Spec: k8sTypes.PodSpec{
			Containers:    containers,
			RestartPolicy: restartPolicy,
		},
	}

	pod, err := obj.client.CoreV1().Pods(options.Namespace).Create(obj.ctx, &newPod, metav1.CreateOptions{})
	if err != nil {
		return k8sTypes.Pod{}, err
	}

	if options.Wait == "" {
		return *pod, nil
	}
	waitOpts := WaitOptions{
		Name:      options.Name,
		Namespace: options.Namespace,
		Status:    Running,
		Timeout:   options.Wait,
	}
	status, err := obj.Wait(waitOpts)
	if err != nil {
		return k8sTypes.Pod{}, err
	}
	if !status {
		return k8sTypes.Pod{}, errors.New("timeout exceeded waiting for pod to be running")
	}

	return obj.Get(options.Name, options.Namespace)
}

// WaitOptions for waiting for a Pod status
//
// Deprecated: No longer used.
type WaitOptions struct {
	Name      string // Pod name
	Namespace string // Namespace where the pod is running
	Status    string // Wait until pod reaches the specified status. Must be one of "Running" or "Succeeded".
	Timeout   string // Timeout for waiting condition to be true
}

// Wait for the Pod to be in a given status up to given timeout and returns a boolean indicating if the status
// was reached. If the pod is Failed returns error.
//
// Deprecated: No longer used.
func (obj *Pods) Wait(options WaitOptions) (bool, error) {
	if options.Status != Running && options.Status != Succeeded {
		return false, errors.New("wait condition must be 'Running' or 'Succeeded'")
	}
	timeout, err := time.ParseDuration(options.Timeout)
	if err != nil {
		return false, err
	}

	return obj.waitForCondition(
		options.Namespace,
		options.Name,
		timeout,
		func(pod *k8sTypes.Pod) (bool, error) {
			if pod.Status.Phase == k8sTypes.PodFailed {
				return false, errors.New("pod has failed")
			}
			if string(pod.Status.Phase) == options.Status {
				return true, nil
			}
			return false, nil
		},
	)
}

// waitForCondition watches a Pod in a namespace until a podConditionChecker is satisfied or a timeout expires
func (obj *Pods) waitForCondition(
	namespace string,
	name string,
	timeout time.Duration,
	checker podConditionChecker,
) (bool, error) {
	selector := fields.Set{
		"metadata.name": name,
	}.AsSelector()

	watcher, err := obj.client.CoreV1().Pods(namespace).Watch(
		obj.ctx,
		metav1.ListOptions{
			FieldSelector: selector.String(),
		},
	)
	if err != nil {
		return false, err
	}
	defer watcher.Stop()

	for {
		select {
		case <-time.After(timeout):
			return false, nil
		case event := <-watcher.ResultChan():
			if event.Type == watch.Error {
				return false, fmt.Errorf("error watching for pod: %v", event.Object)
			}
			if event.Type == watch.Modified {
				pod, isPod := event.Object.(*k8sTypes.Pod)
				if !isPod {
					return false, errors.New("received unknown object while watching for pods")
				}
				condition, err := checker(pod)
				if condition || err != nil {
					return condition, err
				}
			}
		}
	}
}

// Exec executes a non-interactive command described in options and returns the stdout and stderr outputs
//
// Deprecated: No longer used.
func (obj *Pods) Exec(options ExecOptions) (*ExecResult, error) {
	req := obj.client.CoreV1().RESTClient().
		Post().
		Namespace(options.Namespace).
		Resource("pods").
		Name(options.Pod).
		SubResource("exec").
		VersionedParams(&k8sTypes.PodExecOptions{
			Container: options.Container,
			Command:   options.Command,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(obj.config, "POST", req.URL())
	if err != nil {
		return nil, err
	}

	// connect to the command
	var stdout, stderr bytes.Buffer
	stdin := bytes.NewReader(options.Stdin)
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    true,
	})

	if err != nil {
		return nil, err
	}

	result := ExecResult{
		stdout.Bytes(),
		stderr.Bytes(),
	}

	return &result, nil
}

// AddEphemeralContainer adds an ephemeral container to a running pod. The Pod is identified by name and namespace.
// The container is described by options
//
// Deprecated: No longer used.
func (obj *Pods) AddEphemeralContainer(name, namespace string, options EphemeralContainerOptions) error {
	pod, err := obj.Get(name, namespace)
	if err != nil {
		return err
	}
	podJSON, err := json.Marshal(pod)
	if err != nil {
		return err
	}
	container := generateEphemeralContainer(options.ContainerOptions)

	updatedPod := pod.DeepCopy()
	updatedPod.Spec.EphemeralContainers = append(updatedPod.Spec.EphemeralContainers, *container)
	updateJSON, err := json.Marshal(updatedPod)
	if err != nil {
		return err
	}

	patch, err := strategicpatch.CreateTwoWayMergePatch(podJSON, updateJSON, pod)
	if err != nil {
		return err
	}

	_, err = obj.client.CoreV1().Pods(namespace).Patch(
		obj.ctx, pod.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}, "ephemeralcontainers")

	if options.Wait == "" {
		return err
	}

	timeout, err := time.ParseDuration(options.Wait)
	if err != nil {
		return err
	}

	running, err := obj.waitForCondition(
		namespace,
		name,
		timeout,
		checkEphemeralContainerState,
	)
	if err != nil {
		return err
	}
	if !running {
		return errors.New("Ephemeral container has not started after " + options.Wait)
	}
	return nil
}

func generateEphemeralContainer(o ContainerOptions) *k8sTypes.EphemeralContainer {
	capabilities := make([]k8sTypes.Capability, 0)
	for _, capability := range o.Capabilities {
		capabilities = append(capabilities, k8sTypes.Capability(capability))
	}

	return &k8sTypes.EphemeralContainer{
		EphemeralContainerCommon: k8sTypes.EphemeralContainerCommon{
			Name:            o.Name,
			Image:           o.Image,
			ImagePullPolicy: o.PullPolicy,
			Command:         o.Command,
			TTY:             true,
			Stdin:           true,
			SecurityContext: &k8sTypes.SecurityContext{
				Capabilities: &k8sTypes.Capabilities{
					Add: capabilities,
				},
			},
		},
	}
}

func checkEphemeralContainerState(pod *k8sTypes.Pod) (bool, error) {
	if pod.Status.EphemeralContainerStatuses != nil {
		for _, cs := range pod.Status.EphemeralContainerStatuses {
			if cs.State.Running != nil {
				return true, nil
			}
		}
	}

	return false, nil
}
