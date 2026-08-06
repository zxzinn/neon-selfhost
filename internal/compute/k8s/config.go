package k8s

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// RESTConfig loads the REST config from a kubeconfig path (empty = default
// loading rules: $KUBECONFIG, then ~/.kube/config). Exposed separately from
// NewClientset because PortForward needs the *rest.Config itself for the SPDY
// upgrade, not just a kubernetes.Interface built from it.
func RESTConfig(kubeconfig string) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{},
	).ClientConfig()
}

// NewClientset builds a clientset from a kubeconfig path (empty = default
// loading rules: $KUBECONFIG, then ~/.kube/config).
func NewClientset(kubeconfig string) (kubernetes.Interface, error) {
	cfg, err := RESTConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}
