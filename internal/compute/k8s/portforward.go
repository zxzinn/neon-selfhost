package k8s

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForward blocks, forwarding localPort on this machine to the compute
// Pod's 55433 (postgres) for the given branch timeline. It returns when
// stopCh is closed or the tunnel fails.
//
// Unlike Start/Stop/Running, this is not part of the Backend interface: it
// needs a *rest.Config (for the SPDY upgrade), which kubernetes.Interface
// alone does not expose, and the docker backend has no equivalent concept
// (its compute already binds a host port directly).
func (b *Backend) PortForward(ctx context.Context, restConfig *rest.Config, timeline string, localPort int, readyCh chan struct{}, stopCh <-chan struct{}, out, errOut io.Writer) error {
	pod, err := b.findPod(ctx, timeline)
	if err != nil {
		return err
	}

	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return fmt.Errorf("spdy round tripper: %w", err)
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", pod.Namespace, pod.Name)
	hostIP := restConfig.Host
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &url.URL{Scheme: "https", Path: path, Host: trimScheme(hostIP)})

	ports := []string{fmt.Sprintf("%d:%d", localPort, computePGPort)}
	fw, err := portforward.New(dialer, ports, stopCh, readyCh, out, errOut)
	if err != nil {
		return fmt.Errorf("new port forwarder: %w", err)
	}
	return fw.ForwardPorts()
}

// findPod returns the single Running compute Pod for a branch timeline.
func (b *Backend) findPod(ctx context.Context, timeline string) (*corev1.Pod, error) {
	sel := labels.SelectorFromSet(b.baseLabels(timeline)).String()
	list, err := b.cs.CoreV1().Pods(b.cfg.Namespace).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	for _, pod := range list.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return &pod, nil
		}
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("no compute pod found for branch %q (is it started?)", timeline)
	}
	return nil, fmt.Errorf("compute pod for branch %q is not Running yet", timeline)
}

// trimScheme strips a leading "https://" from a REST config host, matching
// the pattern used by client-go's own port-forward examples (the SPDY URL
// below re-adds the scheme explicitly).
func trimScheme(host string) string {
	const prefix = "https://"
	if len(host) > len(prefix) && host[:len(prefix)] == prefix {
		return host[len(prefix):]
	}
	return host
}
