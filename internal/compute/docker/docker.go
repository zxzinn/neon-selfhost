// Package docker implements compute.Backend by running compute containers via
// the `docker` CLI on the local host.
package docker

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/zxzinn/neon-selfhost/internal/compute"
)

// labelManaged tags every container this CLI owns.
const (
	labelManaged = "neon-selfhost=true"
	labelBranch  = "neon-selfhost-branch"
	namePrefix   = "neon-selfhost-branch-"
)

// Backend runs computes as local docker containers.
type Backend struct {
	Image      string // compute image, e.g. built-compute
	Network    string // docker network of the stack
	WrapperDir string // path to compute_wrapper (volume mounts + entrypoint)

	// runner is the command runner; overridable in tests.
	runner runner
}

// New returns a docker Backend.
func New(image, network, wrapperDir string) *Backend {
	return &Backend{Image: image, Network: network, WrapperDir: wrapperDir, runner: execRunner{}}
}

var _ compute.Backend = (*Backend)(nil)

// containerName is the deterministic name for a branch's compute.
func containerName(timeline string) string { return namePrefix + timeline }

// Start launches a compute container bound to (tenant, timeline) on the port.
func (b *Backend) Start(ctx context.Context, s compute.Spec) error {
	name := containerName(s.Timeline)
	args := []string{
		"run", "-d", "--name", name,
		"--label", labelManaged,
		"--label", labelBranch + "=" + s.Timeline,
		"--network", b.Network,
		"-e", "PG_VERSION=" + itoa(s.PGVersion),
		"-e", "TENANT_ID=" + s.Tenant,
		"-e", "TIMELINE_ID=" + s.Timeline,
		"-p", fmt.Sprintf("%d:55433", s.HostPGPort),
		"-v", b.WrapperDir + "/var/db/postgres/configs/:/var/db/postgres/configs/",
		"-v", b.WrapperDir + "/shell/:/shell/",
		"--entrypoint", "/shell/compute.sh",
		b.Image,
	}
	_, err := b.runner.run(ctx, "docker", args...)
	return err
}

// Stop removes the compute container for a timeline.
func (b *Backend) Stop(ctx context.Context, timeline string) error {
	// rm -f exits non-zero if the container is absent; treat that as success.
	out, err := b.runner.run(ctx, "docker", "rm", "-f", containerName(timeline))
	if err != nil && !strings.Contains(out, "No such container") {
		return err
	}
	return nil
}

// Running returns the timeline ids that currently have a compute container.
func (b *Backend) Running(ctx context.Context) (map[string]bool, error) {
	out, err := b.runner.run(ctx, "docker", "ps", "-a",
		"--filter", "label="+labelManaged,
		"--format", "{{.Label \""+labelBranch+"\"}}")
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			set[line] = true
		}
	}
	return set, nil
}

// runner abstracts command execution so tests can inject a fake.
type runner interface {
	run(ctx context.Context, name string, args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out), nil
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
