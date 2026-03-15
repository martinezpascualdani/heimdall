package cmd

import (
	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/config"
	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/output"
	"github.com/spf13/cobra"
)

var (
	cfg         *config.Config
	outputFormat string
)

var rootCmd = &cobra.Command{
	Use:   "heimdallctl",
	Short: "Heimdall CLI – operate dataset, scope, routing, and target services",
	Long:  `Heimdallctl is the official CLI for Heimdall. It talks to dataset-service, scope-service, routing-service, and target-service over HTTP. Use -o json for machine-readable output.`,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format: table | json")
	rootCmd.AddCommand(datasetCmd)
	rootCmd.AddCommand(scopeCmd)
	rootCmd.AddCommand(routingCmd)
	rootCmd.AddCommand(targetCmd)
	rootCmd.AddCommand(statusCmd)
}

// Execute runs the root command. Returns the error from the executed command (caller may os.Exit(1)).
func Execute() error {
	return rootCmd.Execute()
}

func getConfig() *config.Config {
	if cfg == nil {
		cfg = config.Load()
	}
	return cfg
}

func isJSON() bool {
	return output.OutputFormat(outputFormat) == "json"
}
