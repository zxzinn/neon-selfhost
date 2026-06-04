package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/zxzinn/neon-selfhost/internal/compute"
	"github.com/zxzinn/neon-selfhost/internal/pageserver"
	"github.com/zxzinn/neon-selfhost/internal/state"
)

// port range for auto-allocated branch computes.
const (
	portBase = 55434
	portSpan = 100
)

func branchCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "branch",
		Short: "Manage branches",
	}
	c.AddCommand(branchCreateCmd(), branchListCmd(), branchDeleteCmd())
	return c
}

func branchCreateCmd() *cobra.Command {
	var fromName, lsn string
	var port int
	var noCompute bool
	c := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a named branch and start a compute for it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			tenant, err := resolveTenant()
			if err != nil {
				return err
			}
			st, err := state.Load()
			if err != nil {
				return err
			}
			if _, exists := st.Get(tenant, name); exists {
				return fmt.Errorf("branch %q already exists", name)
			}
			ps := psClient()

			// Resolve ancestor timeline: --from <name> if given, else root/main.
			ancestor, err := resolveAncestor(st, ps, tenant, fromName)
			if err != nil {
				return err
			}

			// Allocate a host port unless one was given explicitly.
			if port == 0 {
				port, err = st.AllocPort(portBase, portSpan)
				if err != nil {
					return err
				}
			}

			newID, err := pageserver.GenID()
			if err != nil {
				return err
			}
			tl, err := ps.CreateBranch(tenant, newID, ancestor, lsn, pgVersion)
			if err != nil {
				return err
			}
			fmt.Printf("created branch %q (%s, ancestor %s @ %s)\n", name, tl.TimelineID, tl.AncestorTimelineID, tl.AncestorLSN)

			b := state.Branch{Name: name, TimelineID: tl.TimelineID, Ancestor: tl.AncestorTimelineID, Port: port}
			if !noCompute {
				be, err := backend()
				if err != nil {
					return err
				}
				spec := compute.Spec{Tenant: tenant, Timeline: tl.TimelineID, PGVersion: pgVersion, HostPGPort: port}
				if err := be.Start(cmd.Context(), spec); err != nil {
					return err
				}
				fmt.Printf("compute started: psql -h localhost -p %d -U cloud_admin -d postgres\n", port)
			}
			if err := st.Add(tenant, b); err != nil {
				return err
			}
			return st.Save()
		},
	}
	c.Flags().StringVar(&fromName, "from", "", "ancestor branch name (default: root/main)")
	c.Flags().StringVar(&lsn, "lsn", "", "branch point LSN (default: ancestor's current LSN)")
	c.Flags().IntVar(&port, "port", 0, "host port for the compute (default: auto-allocate)")
	c.Flags().BoolVar(&noCompute, "no-compute", false, "create the timeline only, don't start a compute")
	return c
}

func branchListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List branches",
		RunE: func(cmd *cobra.Command, _ []string) error {
			tenant, err := resolveTenant()
			if err != nil {
				return err
			}
			st, err := state.Load()
			if err != nil {
				return err
			}
			tls, err := psClient().Timelines(tenant)
			if err != nil {
				return err
			}
			be, err := backend()
			if err != nil {
				return err
			}
			running, err := be.Running(cmd.Context())
			if err != nil {
				return err
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTIMELINE\tANCESTOR\tPORT\tCOMPUTE")
			for _, t := range tls {
				anc := t.AncestorTimelineID
				if anc == "" {
					anc = "(root)"
				}
				name, port := "-", "-"
				if b, ok := st.ByTimeline(tenant, t.TimelineID); ok {
					name = b.Name
					port = fmt.Sprintf("%d", b.Port)
				}
				computeState := "-"
				if running[t.TimelineID] {
					computeState = "running"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", name, short(t.TimelineID), short(anc), port, computeState)
			}
			return w.Flush()
		},
	}
}

func branchDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Stop the branch compute and delete the timeline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			tenant, err := resolveTenant()
			if err != nil {
				return err
			}
			st, err := state.Load()
			if err != nil {
				return err
			}

			// Accept either a known name or a raw timeline id.
			timeline := name
			if b, ok := st.Get(tenant, name); ok {
				timeline = b.TimelineID
			}

			be, err := backend()
			if err != nil {
				return err
			}
			_ = be.Stop(cmd.Context(), timeline)
			if err := psClient().DeleteBranch(tenant, timeline); err != nil {
				return err
			}
			st.Remove(tenant, name)
			if err := st.Save(); err != nil {
				return err
			}
			fmt.Printf("deleted branch %q\n", name)
			return nil
		},
	}
}

// resolveAncestor maps --from <name> to a timeline id, or finds root/main.
func resolveAncestor(st *state.Store, ps *pageserver.Client, tenant, fromName string) (string, error) {
	if fromName != "" {
		if b, ok := st.Get(tenant, fromName); ok {
			return b.TimelineID, nil
		}
		return "", fmt.Errorf("unknown ancestor branch %q", fromName)
	}
	tls, err := ps.Timelines(tenant)
	if err != nil {
		return "", err
	}
	for _, t := range tls {
		if t.AncestorTimelineID == "" {
			return t.TimelineID, nil
		}
	}
	if len(tls) > 0 {
		return tls[0].TimelineID, nil
	}
	return "", fmt.Errorf("no timeline to branch from")
}

// short trims an id for display.
func short(id string) string {
	if len(id) > 12 && id != "(root)" {
		return id[:12]
	}
	return id
}
