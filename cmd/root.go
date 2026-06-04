package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zxzinn/neon-selfhost/internal/compute"
	"github.com/zxzinn/neon-selfhost/internal/compute/docker"
	"github.com/zxzinn/neon-selfhost/internal/compute/k8s"
	"github.com/zxzinn/neon-selfhost/internal/pageserver"
)

// Global flags shared by subcommands.
var (
	pageserverURL string
	tenantID      string
	wrapperDir    string
	network       string
	image         string
	pgVersion     int
	backendName   string

	// k8s backend flags
	kubeconfig     string
	namespace      string
	pageserverHost string
	safekeepers    string
)

// version info, injected from main via SetVersion.
var versionString = "dev"

// SetVersion records build version info for the --version flag.
func SetVersion(version, commit string) {
	versionString = version + " (" + commit + ")"
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "neon-selfhost",
		Short: "Manage branches on a self-hosted Neon stack (timelines + compute)",
		Long: "neon-selfhost drives a self-hosted Neon stack: it creates/lists/deletes\n" +
			"branches via the pageserver API and runs one compute per branch.",
		SilenceUsage: true,
		Version:      versionString,
	}

	pf := root.PersistentFlags()
	pf.StringVar(&pageserverURL, "pageserver", "http://localhost:9898", "pageserver HTTP endpoint")
	pf.StringVar(&tenantID, "tenant", "", "tenant id (defaults to the first tenant)")
	pf.StringVar(&wrapperDir, "wrapper-dir", "/tmp/neon-compose/docker-compose/compute_wrapper", "path to compute_wrapper")
	pf.StringVar(&network, "network", "docker-compose_default", "docker network of the stack")
	pf.StringVar(&image, "image", "built-compute", "compute image")
	pf.IntVar(&pgVersion, "pg-version", 16, "postgres major version")
	pf.StringVar(&backendName, "backend", "docker", "compute backend (docker, k8s)")
	pf.StringVar(&kubeconfig, "kubeconfig", "", "kubeconfig path (k8s backend; default: $KUBECONFIG or ~/.kube/config)")
	pf.StringVar(&namespace, "namespace", "default", "namespace for computes (k8s backend)")
	pf.StringVar(&pageserverHost, "pageserver-host", "pageserver.neon.svc.cluster.local", "in-cluster pageserver host (k8s backend)")
	pf.StringVar(&safekeepers, "safekeepers", "", "comma-separated safekeeper list, host:port (k8s backend)")

	root.AddCommand(initCmd(), branchCmd())
	return root
}

// Execute is the entrypoint called by main.
func Execute() error {
	return rootCmd().Execute()
}

// helpers shared by subcommands

func psClient() *pageserver.Client { return pageserver.New(pageserverURL) }

// backend resolves the configured compute backend. Adding a k8s backend means
// adding a case here; the rest of the cmd layer depends only on the interface.
func backend() (compute.Backend, error) {
	switch backendName {
	case "docker":
		return docker.New(image, network, wrapperDir), nil
	case "k8s":
		cs, err := k8s.NewClientset(kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("k8s: %w", err)
		}
		return k8s.New(cs, k8s.Config{
			Namespace:      namespace,
			Image:          image,
			PGVersion:      pgVersion,
			PageserverHost: pageserverHost,
			Safekeepers:    safekeepers,
		}), nil
	default:
		return nil, fmt.Errorf("unknown backend %q (supported: docker, k8s)", backendName)
	}
}

// resolveTenant returns the explicit --tenant flag or the first tenant.
func resolveTenant() (string, error) {
	if tenantID != "" {
		return tenantID, nil
	}
	return psClient().DefaultTenant()
}
