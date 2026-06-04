package k8s

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/zxzinn/neon-selfhost/internal/compute"
)

func newTestBackend() (*Backend, *fake.Clientset) {
	cs := fake.NewSimpleClientset()
	b := New(cs, Config{
		Namespace:      "neon",
		Image:          "registry/compute:16",
		PGVersion:      16,
		PageserverHost: "pageserver.neon.svc",
		Safekeepers:    "sk-0:5454,sk-1:5454",
	})
	return b, cs
}

func TestStartCreatesDeploymentAndService(t *testing.T) {
	b, cs := newTestBackend()
	spec := compute.Spec{Tenant: "ten", Timeline: "tl1", PGVersion: 16, HostPGPort: 0}
	if err := b.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}

	dep, err := cs.AppsV1().Deployments("neon").Get(context.Background(), "neon-compute-tl1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("deployment not created: %v", err)
	}
	// labels
	if dep.Labels[labelManaged] != managedValue || dep.Labels[labelBranch] != "tl1" {
		t.Fatalf("bad labels: %v", dep.Labels)
	}
	// env
	c := dep.Spec.Template.Spec.Containers[0]
	if c.Image != "registry/compute:16" {
		t.Errorf("image = %q", c.Image)
	}
	env := map[string]string{}
	for _, e := range c.Env {
		env[e.Name] = e.Value
	}
	if env["TENANT_ID"] != "ten" || env["TIMELINE_ID"] != "tl1" {
		t.Errorf("tenant/timeline env wrong: %v", env)
	}
	if env["PAGESERVER_HOST"] != "pageserver.neon.svc" {
		t.Errorf("PAGESERVER_HOST = %q", env["PAGESERVER_HOST"])
	}
	if env["SAFEKEEPERS"] != "sk-0:5454,sk-1:5454" {
		t.Errorf("SAFEKEEPERS = %q", env["SAFEKEEPERS"])
	}
	if env["PG_VERSION"] != "16" {
		t.Errorf("PG_VERSION = %q", env["PG_VERSION"])
	}

	svc, err := cs.CoreV1().Services("neon").Get(context.Background(), "neon-compute-tl1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("service not created: %v", err)
	}
	if svc.Spec.Selector[labelBranch] != "tl1" {
		t.Errorf("service selector wrong: %v", svc.Spec.Selector)
	}
	if svc.Spec.Ports[0].Port != 5432 {
		t.Errorf("service port = %d", svc.Spec.Ports[0].Port)
	}
}

func TestStartIsIdempotent(t *testing.T) {
	b, _ := newTestBackend()
	spec := compute.Spec{Tenant: "ten", Timeline: "tl1", PGVersion: 16}
	if err := b.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	// second Start should not error on AlreadyExists
	if err := b.Start(context.Background(), spec); err != nil {
		t.Fatalf("re-Start should be idempotent, got %v", err)
	}
}

func TestStopDeletesBoth(t *testing.T) {
	b, cs := newTestBackend()
	spec := compute.Spec{Tenant: "ten", Timeline: "tl1", PGVersion: 16}
	if err := b.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if err := b.Stop(context.Background(), "tl1"); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.AppsV1().Deployments("neon").Get(context.Background(), "neon-compute-tl1", metav1.GetOptions{}); err == nil {
		t.Error("deployment still exists after Stop")
	}
	if _, err := cs.CoreV1().Services("neon").Get(context.Background(), "neon-compute-tl1", metav1.GetOptions{}); err == nil {
		t.Error("service still exists after Stop")
	}
}

func TestStopMissingIsNoError(t *testing.T) {
	b, _ := newTestBackend()
	if err := b.Stop(context.Background(), "does-not-exist"); err != nil {
		t.Fatalf("Stop on missing should be no-op, got %v", err)
	}
}

func TestRunning(t *testing.T) {
	b, _ := newTestBackend()
	for _, tl := range []string{"tl1", "tl2"} {
		if err := b.Start(context.Background(), compute.Spec{Tenant: "ten", Timeline: tl, PGVersion: 16}); err != nil {
			t.Fatal(err)
		}
	}
	set, err := b.Running(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !set["tl1"] || !set["tl2"] || len(set) != 2 {
		t.Fatalf("Running = %v; want {tl1,tl2}", set)
	}
}

func TestNamespaceDefaultsToDefault(t *testing.T) {
	cs := fake.NewSimpleClientset()
	b := New(cs, Config{Image: "x"}) // no namespace
	if b.cfg.Namespace != "default" {
		t.Fatalf("namespace = %q; want default", b.cfg.Namespace)
	}
}

var _ compute.Backend = (*Backend)(nil)
