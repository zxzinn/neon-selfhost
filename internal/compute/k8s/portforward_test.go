package k8s

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFindPodReturnsRunningPod(t *testing.T) {
	b, cs := newTestBackend()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "neon-compute-tl1-xyz", Namespace: "neon", Labels: b.baseLabels("tl1")},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	if _, err := cs.CoreV1().Pods("neon").Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	got, err := b.findPod(context.Background(), "tl1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "neon-compute-tl1-xyz" {
		t.Errorf("findPod name = %q; want neon-compute-tl1-xyz", got.Name)
	}
}

func TestFindPodIgnoresOtherBranches(t *testing.T) {
	b, cs := newTestBackend()
	other := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "neon-compute-tl2-xyz", Namespace: "neon", Labels: b.baseLabels("tl2")},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	if _, err := cs.CoreV1().Pods("neon").Create(context.Background(), other, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	if _, err := b.findPod(context.Background(), "tl1"); err == nil {
		t.Fatal("expected error when no pod matches the requested branch")
	}
}

func TestFindPodErrorsWhenNotRunning(t *testing.T) {
	b, cs := newTestBackend()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "neon-compute-tl1-xyz", Namespace: "neon", Labels: b.baseLabels("tl1")},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}
	if _, err := cs.CoreV1().Pods("neon").Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	if _, err := b.findPod(context.Background(), "tl1"); err == nil {
		t.Fatal("expected error for a Pending (not Running) pod")
	}
}

func TestTrimScheme(t *testing.T) {
	cases := map[string]string{
		"https://example.com:443": "example.com:443",
		"example.com:443":         "example.com:443",
	}
	for in, want := range cases {
		if got := trimScheme(in); got != want {
			t.Errorf("trimScheme(%q) = %q; want %q", in, got, want)
		}
	}
}
