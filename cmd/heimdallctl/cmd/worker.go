package cmd

import (
	"context"
	"os"

	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/client"
	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/output"
	"github.com/spf13/cobra"
)

var (
	workerStatus string
	workerLimit  int
	workerOffset int
	workerJobLimit int
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
	Long:  `List jobs in assigned or running state for this worker (current load). For debugging which jobs a worker is processing.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkerJobs,
}

func init() {
	workerListCmd.Flags().StringVar(&workerStatus, "status", "", "Filter by status: online|offline")
	workerListCmd.Flags().IntVar(&workerLimit, "limit", 100, "Max workers to return")
	workerListCmd.Flags().IntVar(&workerOffset, "offset", 0, "Offset for pagination")

	workerJobsCmd.Flags().IntVar(&workerJobLimit, "limit", 100, "Max jobs to return")

	workerCmd.AddCommand(workerListCmd)
	workerCmd.AddCommand(workerGetCmd)
	workerCmd.AddCommand(workerJobsCmd)
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
	resp, err := cl.WorkerListJobs(ctx, args[0], workerJobLimit)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, resp)
	}
	output.PrintExecutionJobs(os.Stdout, resp.Items, resp.Total, resp.Limit, 0, "Worker jobs")
	return nil
}
