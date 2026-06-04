package docker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zxzinn/neon-selfhost/internal/compute"
)

// fakeRunner records calls and returns canned output/errors.
type fakeRunner struct {
	calls  [][]string
	out    string
	err    error
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.out, f.err
}

func newWithRunner(r runner) *Backend {
	return &Backend{Image: "built-compute", Network: "net", WrapperDir: "/w", runner: r}
}

func TestStartArgs(t *testing.T) {
	f := &fakeRunner{}
	b := newWithRunner(f)
	err := b.Start(context.Background(), compute.Spec{
		Tenant: "ten", Timeline: "tl1", PGVersion: 16, HostPGPort: 55440,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("expected 1 docker call, got %d", len(f.calls))
	}
	got := strings.Join(f.calls[0], " ")
	for _, want := range []string{
		"docker run -d",
		"--name neon-selfhost-branch-tl1",
		"--label neon-selfhost=true",
		"--label neon-selfhost-branch=tl1",
		"--network net",
		"-e TENANT_ID=ten",
		"-e TIMELINE_ID=tl1",
		"-e PG_VERSION=16",
		"-p 55440:55433",
		"--entrypoint /shell/compute.sh",
		"built-compute",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("docker args missing %q\nfull: %s", want, got)
		}
	}
}

func TestStopIgnoresMissingContainer(t *testing.T) {
	f := &fakeRunner{out: "Error: No such container: neon-selfhost-branch-x", err: errors.New("exit 1")}
	b := newWithRunner(f)
	if err := b.Stop(context.Background(), "x"); err != nil {
		t.Fatalf("Stop should ignore missing container, got %v", err)
	}
}

func TestStopPropagatesRealError(t *testing.T) {
	f := &fakeRunner{out: "Cannot connect to the Docker daemon", err: errors.New("exit 1")}
	b := newWithRunner(f)
	if err := b.Stop(context.Background(), "x"); err == nil {
		t.Fatal("Stop should propagate non-missing errors")
	}
}

func TestRunningParsesLabels(t *testing.T) {
	f := &fakeRunner{out: "tl1\ntl2\n\n"}
	b := newWithRunner(f)
	set, err := b.Running(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !set["tl1"] || !set["tl2"] || len(set) != 2 {
		t.Fatalf("Running = %v; want {tl1,tl2}", set)
	}
}

func TestRunningEmpty(t *testing.T) {
	f := &fakeRunner{out: "\n"}
	b := newWithRunner(f)
	set, err := b.Running(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != 0 {
		t.Fatalf("Running = %v; want empty", set)
	}
}

// compile-time assertion that Backend satisfies the interface.
var _ compute.Backend = (*Backend)(nil)
