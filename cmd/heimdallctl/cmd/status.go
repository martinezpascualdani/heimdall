package cmd

import (
	"context"
	"os"

	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/client"
	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/output"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check health of dataset, scope, and routing services (GET /health)",
	RunE:  runStatus,
}

func init() {
	// no flags
}

func runStatus(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	dataset := cl.DatasetHealth(ctx)
	scope := cl.ScopeHealth(ctx)
	routing := cl.RoutingHealth(ctx)
	if isJSON() {
		m := map[string]interface{}{
			"dataset": map[string]interface{}{"ok": dataset.OK, "error": dataset.Error},
			"scope":   map[string]interface{}{"ok": scope.OK, "error": scope.Error},
			"routing": map[string]interface{}{"ok": routing.OK, "error": routing.Error},
		}
		return output.PrintJSON(os.Stdout, m)
	}
	output.PrintStatus(os.Stdout, dataset, scope, routing)
	return nil
}
