package cmd

import (
	"context"
	"os"

	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/client"
	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/output"
	"github.com/spf13/cobra"
)

var (
	scopeDatasetID     string
	scopeAddressFamily string
	scopeLimit         int
	scopeOffset        int
)

var scopeCmd = &cobra.Command{
	Use:   "scope",
	Short: "Interact with scope-service (RIR inventory, IP→country)",
}

var scopeSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync scope imports from dataset-service",
	RunE:  runScopeSync,
}

var scopeByIPCmd = &cobra.Command{
	Use:   "by-ip [ip]",
	Short: "Resolve IP to country",
	Args:  cobra.ExactArgs(1),
	RunE:  runScopeByIP,
}

var scopeCountryCmd = &cobra.Command{
	Use:   "country",
	Short: "Country-scoped queries (summary, blocks, asns, asn-summary, datasets)",
}

var scopeCountrySummaryCmd = &cobra.Command{
	Use:   "summary [cc]",
	Short: "Summary counts for country",
	Args:  cobra.ExactArgs(1),
	RunE:  runScopeCountrySummary,
}

var scopeCountryBlocksCmd = &cobra.Command{
	Use:   "blocks [cc]",
	Short: "List blocks for country",
	Args:  cobra.ExactArgs(1),
	RunE:  runScopeCountryBlocks,
}

var scopeCountryASNsCmd = &cobra.Command{
	Use:   "asns [cc]",
	Short: "List ASN ranges for country",
	Args:  cobra.ExactArgs(1),
	RunE:  runScopeCountryASNs,
}

var scopeCountryASNSummaryCmd = &cobra.Command{
	Use:   "asn-summary [cc]",
	Short: "ASN summary for country",
	Args:  cobra.ExactArgs(1),
	RunE:  runScopeCountryASNSummary,
}

var scopeCountryDatasetsCmd = &cobra.Command{
	Use:   "datasets [cc]",
	Short: "List datasets that have blocks for country",
	Args:  cobra.ExactArgs(1),
	RunE:  runScopeCountryDatasets,
}

func init() {
	scopeCmd.AddCommand(scopeSyncCmd)

	scopeByIPCmd.Flags().StringVar(&scopeDatasetID, "dataset-id", "", "Optional dataset ID")
	scopeCmd.AddCommand(scopeByIPCmd)

	scopeCountryCmd.PersistentFlags().StringVar(&scopeDatasetID, "dataset-id", "", "Optional dataset ID")
	scopeCountrySummaryCmd.Flags().StringVar(&scopeAddressFamily, "address-family", "", "Optional: all|ipv4|ipv6")
	scopeCountryBlocksCmd.Flags().StringVar(&scopeAddressFamily, "address-family", "", "Optional: all|ipv4|ipv6")
	scopeCountryBlocksCmd.Flags().IntVar(&scopeLimit, "limit", 100, "Limit")
	scopeCountryBlocksCmd.Flags().IntVar(&scopeOffset, "offset", 0, "Offset")
	scopeCountryASNsCmd.Flags().IntVar(&scopeLimit, "limit", 100, "Limit")
	scopeCountryASNsCmd.Flags().IntVar(&scopeOffset, "offset", 0, "Offset")

	scopeCountryCmd.AddCommand(scopeCountrySummaryCmd)
	scopeCountryCmd.AddCommand(scopeCountryBlocksCmd)
	scopeCountryCmd.AddCommand(scopeCountryASNsCmd)
	scopeCountryCmd.AddCommand(scopeCountryASNSummaryCmd)
	scopeCountryCmd.AddCommand(scopeCountryDatasetsCmd)
	scopeCmd.AddCommand(scopeCountryCmd)
}

func runScopeSync(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.ScopeSync(ctx)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintScopeSync(os.Stdout, r)
	return nil
}

func runScopeByIP(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.ScopeByIP(ctx, args[0], scopeDatasetID)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintIPResolve(os.Stdout, r)
	return nil
}

func runScopeCountrySummary(c *cobra.Command, args []string) error {
	cc := args[0]
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.ScopeCountrySummary(ctx, cc, scopeDatasetID, scopeAddressFamily)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCountrySummary(os.Stdout, r)
	return nil
}

func runScopeCountryBlocks(c *cobra.Command, args []string) error {
	cc := args[0]
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.ScopeCountryBlocks(ctx, cc, scopeDatasetID, scopeAddressFamily, scopeLimit, scopeOffset)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCountryBlocks(os.Stdout, r)
	return nil
}

func runScopeCountryASNs(c *cobra.Command, args []string) error {
	cc := args[0]
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.ScopeCountryASNs(ctx, cc, scopeDatasetID, scopeLimit, scopeOffset)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCountryASNs(os.Stdout, r)
	return nil
}

func runScopeCountryASNSummary(c *cobra.Command, args []string) error {
	cc := args[0]
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.ScopeCountryASNSummary(ctx, cc, scopeDatasetID)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCountryASNSummary(os.Stdout, r)
	return nil
}

func runScopeCountryDatasets(c *cobra.Command, args []string) error {
	cc := args[0]
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.ScopeCountryDatasets(ctx, cc)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCountryDatasets(os.Stdout, r)
	return nil
}
	// In Cobra, when you run "scope country ES summary", the root args are [], scope args [], country args [ES], summary args [].
