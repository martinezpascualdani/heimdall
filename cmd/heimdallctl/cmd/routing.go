package cmd

import (
	"context"
	"os"
	"strconv"

	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/client"
	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/output"
	"github.com/spf13/cobra"
)

var (
	routingDatasetID string
	routingLimit      int
	routingOffset     int
)

var routingCmd = &cobra.Command{
	Use:   "routing",
	Short: "Interact with routing-service (BGP, IP→ASN, ASN metadata)",
}

var routingSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync routing imports from dataset-service (CAIDA pfx2as + as-org)",
	RunE:  runRoutingSync,
}

var routingByIPCmd = &cobra.Command{
	Use:   "by-ip [ip]",
	Short: "Resolve IP to ASN (longest-prefix match)",
	Args:  cobra.ExactArgs(1),
	RunE:  runRoutingByIP,
}

var routingAsnCmd = &cobra.Command{
	Use:   "asn [asn]",
	Short: "Get ASN metadata (name, org)",
	Args:  cobra.ExactArgs(1),
	RunE:  runRoutingASNMeta,
}

var routingAsnPrefixesCmd = &cobra.Command{
	Use:   "prefixes [asn]",
	Short: "List prefixes where primary_asn = ASN",
	Args:  cobra.ExactArgs(1),
	RunE:  runRoutingASNPrefixes,
}

func init() {
	routingCmd.AddCommand(routingSyncCmd)

	routingByIPCmd.Flags().StringVar(&routingDatasetID, "dataset-id", "", "Optional dataset ID (routing snapshot)")
	routingCmd.AddCommand(routingByIPCmd)

	routingCmd.AddCommand(routingAsnCmd)
	routingAsnPrefixesCmd.Flags().StringVar(&routingDatasetID, "dataset-id", "", "Optional dataset ID")
	routingAsnPrefixesCmd.Flags().IntVar(&routingLimit, "limit", 100, "Limit")
	routingAsnPrefixesCmd.Flags().IntVar(&routingOffset, "offset", 0, "Offset")
	routingAsnCmd.AddCommand(routingAsnPrefixesCmd)
}

func runRoutingSync(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.RoutingSync(ctx)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintRoutingSync(os.Stdout, r)
	return nil
}

func runRoutingByIP(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.RoutingByIP(ctx, args[0], routingDatasetID)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintRoutingByIP(os.Stdout, r)
	return nil
}

func runRoutingASNMeta(cmd *cobra.Command, args []string) error {
	asn, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || asn < 0 {
		return err
	}
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.RoutingASNMeta(ctx, asn)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintRoutingASNMeta(os.Stdout, r)
	return nil
}

func runRoutingASNPrefixes(cmd *cobra.Command, args []string) error {
	asn, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || asn < 0 {
		return err
	}
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.RoutingASNPrefixes(ctx, asn, routingDatasetID, routingLimit, routingOffset)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintASNPrefixes(os.Stdout, r)
	return nil
}
