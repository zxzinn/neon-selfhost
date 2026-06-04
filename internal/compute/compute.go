// Package compute manages Neon compute instances, one per branch.
//
// Backend abstracts *where* a compute runs (local docker today, k8s later) so
// the cmd layer never depends on a concrete runtime. Each branch maps to one
// compute, addressed by its timeline id.
package compute

import "context"

// Spec describes a compute to start for a branch.
type Spec struct {
	Tenant     string // tenant (project) id
	Timeline   string // timeline (branch) id this compute serves
	PGVersion  int    // postgres major version
	HostPGPort int    // host-side port to reach postgres on
}

// Backend runs computes on some runtime.
type Backend interface {
	// Start launches a compute for the spec. Idempotency is the backend's
	// concern; callers should not assume a no-op on re-create.
	Start(ctx context.Context, s Spec) error
	// Stop removes the compute for a timeline. Missing compute is not an error.
	Stop(ctx context.Context, timeline string) error
	// Running returns the set of timeline ids that currently have a compute.
	Running(ctx context.Context) (map[string]bool, error)
}
