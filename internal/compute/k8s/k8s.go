// Package k8s implements compute.Backend by running each branch's compute as a
// Deployment (+ Service) in a Kubernetes cluster.
//
// It assumes the storage layer (pageserver, safekeeper) also runs in-cluster
// and is reachable via service DNS; the compute is configured with TENANT_ID /
// TIMELINE_ID env vars, exactly like the docker compute wrapper.
package k8s

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	"github.com/zxzinn/neon-selfhost/internal/compute"
)

// Labels applied to every object this backend owns.
const (
	labelManaged = "app.kubernetes.io/managed-by"
	managedValue = "neon-selfhost"
	labelBranch  = "neon-selfhost/branch"
)

// Config configures the k8s backend.
type Config struct {
	Namespace string // namespace to create computes in
	Image     string // compute image
	PGVersion int    // postgres major version
	// PageserverHost is the in-cluster host the compute connects to,
	// e.g. "pageserver.neon.svc.cluster.local".
	PageserverHost string
	// Safekeepers is the comma-separated safekeeper list for neon.safekeepers,
	// e.g. "safekeeper-0.sk:5454,safekeeper-1.sk:5454".
	Safekeepers string
}

// Backend creates compute Deployments and Services.
type Backend struct {
	cs  kubernetes.Interface
	cfg Config
}

// New builds a Backend from an existing clientset (injected for testability).
func New(cs kubernetes.Interface, cfg Config) *Backend {
	if cfg.Namespace == "" {
		cfg.Namespace = "default"
	}
	return &Backend{cs: cs, cfg: cfg}
}

var _ compute.Backend = (*Backend)(nil)

// objName is the deterministic name for a branch's Deployment/Service.
// Timeline ids are 32 hex chars, safe as a DNS-1123 name suffix.
func objName(timeline string) string { return "neon-compute-" + timeline }

func (b *Backend) baseLabels(timeline string) map[string]string {
	return map[string]string{
		labelManaged: managedValue,
		labelBranch:  timeline,
	}
}

// Start creates a Deployment + Service for the branch. Already-exists is not an
// error (idempotent re-create).
func (b *Backend) Start(ctx context.Context, s compute.Spec) error {
	name := objName(s.Timeline)
	lbls := b.baseLabels(s.Timeline)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: b.cfg.Namespace, Labels: lbls},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{MatchLabels: lbls},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: lbls},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "compute",
						Image: b.cfg.Image,
						Env: []corev1.EnvVar{
							{Name: "PG_VERSION", Value: itoa(b.cfg.PGVersion)},
							{Name: "TENANT_ID", Value: s.Tenant},
							{Name: "TIMELINE_ID", Value: s.Timeline},
							{Name: "PAGESERVER_HOST", Value: b.cfg.PageserverHost},
							{Name: "SAFEKEEPERS", Value: b.cfg.Safekeepers},
						},
						Ports: []corev1.ContainerPort{{ContainerPort: 55433, Name: "postgres"}},
					}},
				},
			},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: b.cfg.Namespace, Labels: lbls},
		Spec: corev1.ServiceSpec{
			Selector: lbls,
			Ports: []corev1.ServicePort{{
				Name:       "postgres",
				Port:       5432,
				TargetPort: intstr.FromInt32(55433),
			}},
		},
	}

	if _, err := b.cs.AppsV1().Deployments(b.cfg.Namespace).Create(ctx, dep, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create deployment: %w", err)
	}
	if _, err := b.cs.CoreV1().Services(b.cfg.Namespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create service: %w", err)
	}
	return nil
}

// Stop deletes the Deployment + Service for a branch. Missing objects are not
// an error.
func (b *Backend) Stop(ctx context.Context, timeline string) error {
	name := objName(timeline)
	if err := b.cs.AppsV1().Deployments(b.cfg.Namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete deployment: %w", err)
	}
	if err := b.cs.CoreV1().Services(b.cfg.Namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

// Running returns the set of branch timeline ids that have a Deployment.
func (b *Backend) Running(ctx context.Context) (map[string]bool, error) {
	sel := labels.SelectorFromSet(map[string]string{labelManaged: managedValue}).String()
	list, err := b.cs.AppsV1().Deployments(b.cfg.Namespace).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, d := range list.Items {
		if tl := d.Labels[labelBranch]; tl != "" {
			set[tl] = true
		}
	}
	return set, nil
}

func int32Ptr(i int32) *int32 { return &i }
func itoa(n int) string       { return fmt.Sprintf("%d", n) }
