package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/client"
	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/output"
	"github.com/spf13/cobra"
)

var (
	workerStatus       string
	workerLimit        int
	workerOffset       int
	workerJobLimit     int
	workerMaxConcurrency int
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Manage workers (execution-service)",
	Long:  `List workers, get worker by ID, list jobs currently assigned/running on a worker. Useful for debugging which worker is doing what.`,
}

var workerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workers",
	Long:  `List registered workers. Optionally filter by status (online|offline).`,
	RunE:  runWorkerList,
}

var workerGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get worker by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkerGet,
}

var workerJobsCmd = &cobra.Command{
	Use:   "jobs [worker-id]",
	Short: "List jobs currently assigned to a worker",
	Long:  `List jobs in assigned or running state for this worker. If worker-id is omitted and there is exactly one online worker, uses that one.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runWorkerJobs,
}

var workerUpdateCmd = &cobra.Command{
	Use:   "update [worker-id]",
	Short: "Update worker max concurrency",
	Long:  `Sets max_concurrency for the worker (jobs in parallel). If worker-id is omitted and there is exactly one online worker, uses that one.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runWorkerUpdate,
}

func init() {
	workerListCmd.Flags().StringVar(&workerStatus, "status", "", "Filter by status: online|offline")
	workerListCmd.Flags().IntVar(&workerLimit, "limit", 100, "Max workers to return")
	workerListCmd.Flags().IntVar(&workerOffset, "offset", 0, "Offset for pagination")

	workerJobsCmd.Flags().IntVar(&workerJobLimit, "limit", 100, "Max jobs to return")

	workerUpdateCmd.Flags().IntVar(&workerMaxConcurrency, "max-concurrency", 0, "Max jobs to run in parallel (required)")

	workerCmd.AddCommand(workerListCmd)
	workerCmd.AddCommand(workerGetCmd)
	workerCmd.AddCommand(workerJobsCmd)
	workerCmd.AddCommand(workerUpdateCmd)
	rootCmd.AddCommand(workerCmd)
}

func runWorkerList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cl := client.New(getConfig())
	resp, err := cl.WorkerList(ctx, workerStatus, workerLimit, workerOffset)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, resp)
	}
	output.PrintWorkerList(os.Stdout, resp)
	return nil
}

func runWorkerGet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cl := client.New(getConfig())
	w, err := cl.WorkerGet(ctx, args[0])
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, w)
	}
	output.PrintWorker(os.Stdout, w)
	return nil
}

func runWorkerJobs(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cl := client.New(getConfig())
	workerID := ""
	if len(args) > 0 {
		workerID = args[0]
	} else {
		// No arg: use first online worker if exactly one
		list, err := cl.WorkerList(ctx, "online", 10, 0)
		if err != nil {
			return err
		}
		if len(list.Items) == 0 {
			return fmt.Errorf("no online worker; specify worker-id (use 'heimdallctl worker list' to see IDs)")
		}
		if len(list.Items) > 1 {
			return fmt.Errorf("multiple online workers; specify worker-id (use 'heimdallctl worker list' to see IDs)")
		}
		workerID = list.Items[0].ID
	}
	resp, err := cl.WorkerListJobs(ctx, workerID, workerJobLimit)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, resp)
	}
	output.PrintExecutionJobs(os.Stdout, resp.Items, resp.Total, resp.Limit, 0, "Worker jobs")
	return nil
}

func runWorkerUpdate(cmd *cobra.Command, args []string) error {
	if workerMaxConcurrency <= 0 {
		return fmt.Errorf("--max-concurrency is required and must be >= 1")
	}
	ctx := context.Background()
	cl := client.New(getConfig())
	workerID := ""
	if len(args) > 0 {
		workerID = args[0]
	} else {
		list, err := cl.WorkerList(ctx, "online", 10, 0)
		if err != nil {
			return err
		}
		if len(list.Items) == 0 {
			return fmt.Errorf("no online worker; specify worker-id")
		}
		if len(list.Items) > 1 {
			return fmt.Errorf("multiple online workers; specify worker-id")
		}
		workerID = list.Items[0].ID
	}
	if err := cl.WorkerUpdateMaxConcurrency(ctx, workerID, workerMaxConcurrency); err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, map[string]interface{}{"worker_id": workerID, "max_concurrency": workerMaxConcurrency})
	}
	fmt.Fprintf(os.Stdout, "  Worker %s max_concurrency set to %d.\n", workerID, workerMaxConcurrency)
	return nil
}
