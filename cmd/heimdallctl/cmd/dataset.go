package cmd

import (
	"context"
	"os"

	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/client"
	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/output"
	"github.com/spf13/cobra"
)

var (
	datasetRegistry string
	datasetSource   string
	datasetSourceType string
)

var datasetCmd = &cobra.Command{
	Use:   "dataset",
	Short: "Interact with dataset-service (fetch, list, get)",
}

var datasetFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch RIR or CAIDA dataset",
	Long:  "Use --registry=ripencc|arin|apnic|lacnic|afrinic|all for RIR, or --source=caida_pfx2as_ipv4|caida_pfx2as_ipv6|caida_as_org for CAIDA.",
	RunE:  runDatasetFetch,
}

var datasetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List dataset versions",
	RunE:  runDatasetList,
}

var datasetGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get metadata for one dataset",
	Args:  cobra.ExactArgs(1),
	RunE:  runDatasetGet,
}

func init() {
	datasetFetchCmd.Flags().StringVar(&datasetRegistry, "registry", "", "RIR: ripencc|arin|apnic|lacnic|afrinic|all")
	datasetFetchCmd.Flags().StringVar(&datasetSource, "source", "", "CAIDA: caida_pfx2as_ipv4|caida_pfx2as_ipv6|caida_as_org")
	datasetCmd.AddCommand(datasetFetchCmd)

	datasetListCmd.Flags().StringVar(&datasetSource, "source", "", "Filter by source")
	datasetListCmd.Flags().StringVar(&datasetSourceType, "source-type", "", "Filter by source type: rir|caida")
	datasetCmd.AddCommand(datasetListCmd)

	datasetCmd.AddCommand(datasetGetCmd)
}

func runDatasetFetch(cmd *cobra.Command, args []string) error {
	if datasetRegistry == "" && datasetSource == "" {
		datasetRegistry = "all"
	}
	cl := client.New(getConfig())
	ctx := context.Background()
	result, err := cl.DatasetFetch(ctx, datasetRegistry, datasetSource)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, result)
	}
	output.PrintFetchResult(os.Stdout, result)
	return nil
}

func runDatasetList(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	list, err := cl.DatasetList(ctx, datasetSource, datasetSourceType)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, list)
	}
	output.PrintDatasetList(os.Stdout, list)
	return nil
}

func runDatasetGet(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	d, err := cl.DatasetGet(ctx, args[0])
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, d)
	}
	output.PrintDatasetGet(os.Stdout, d)
	return nil
}
