package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/zxzinn/neon-selfhost/internal/pageserver"
)

func initCmd() *cobra.Command {
	var timeout time.Duration
	c := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a tenant + root timeline on an empty stack",
		Long: "init creates the initial tenant and root (main) timeline so that\n" +
			"`branch create` has something to branch from. Run once after deploying\n" +
			"a fresh storage stack (e.g. the Helm chart).",
		RunE: func(_ *cobra.Command, _ []string) error {
			ps := psClient()

			// If a tenant already exists, don't create a second one.
			if existing, err := ps.Tenants(); err == nil && len(existing) > 0 {
				tenant := existing[0].ID
				tls, _ := ps.Timelines(tenant)
				if len(tls) > 0 {
					fmt.Printf("already initialized: tenant %s with %d timeline(s)\n", tenant, len(tls))
					return nil
				}
				return createRoot(ps, tenant)
			}

			tenant, err := pageserver.GenID()
			if err != nil {
				return err
			}
			fmt.Printf("creating tenant %s\n", tenant)
			if err := ps.CreateTenant(tenant); err != nil {
				return err
			}
			fmt.Printf("waiting for tenant to become active (timeout %s)...\n", timeout)
			if err := ps.WaitTenantActive(tenant, timeout); err != nil {
				return err
			}
			return createRoot(ps, tenant)
		},
	}
	c.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "how long to wait for the tenant to become active")
	return c
}

// createRoot creates the root (main) timeline for a tenant.
func createRoot(ps *pageserver.Client, tenant string) error {
	id, err := pageserver.GenID()
	if err != nil {
		return err
	}
	tl, err := ps.CreateBranch(tenant, id, "", "", pgVersion)
	if err != nil {
		return err
	}
	fmt.Printf("created root timeline %s on tenant %s\n", tl.TimelineID, tenant)
	fmt.Println("ready: now run `branch create <name>`")
	return nil
}
