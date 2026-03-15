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
	executionRunID     string
	executionCampaignID string
	executionStatus    string
	executionLimit     int
	executionOffset    int
	executionJobLimit  int
	executionJobOffset int
)

var executionCmd = &cobra.Command{
	Use:   "execution",
	Short: "Manage executions and jobs (execution-service)",
	Long:  `List executions, get execution details, list jobs of an execution, requeue failed jobs, or cancel an execution. Useful for debugging the execution plane.`,
}

var executionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List executions with optional filters",
	Long:  `List executions. Filter by run_id, campaign_id, or status (planning|running|completed|failed|canceled).`,
	RunE:  runExecutionList,
}

var executionGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get execution by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runExecutionGet,
}

var executionJobsCmd = &cobra.Command{
	Use:   "jobs [execution-id]",
	Short: "List jobs of an execution",
	Long:  `List jobs for an execution (pending, assigned, running, completed, failed, canceled). Use for debugging job distribution and status.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runExecutionJobs,
}

var executionRequeueCmd = &cobra.Command{
	Use:   "requeue [id]",
	Short: "Requeue all failed jobs of an execution",
	Long:  `Puts failed jobs (with attempt < max_attempts) back to pending so workers can claim them again.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runExecutionRequeue,
}

var executionCancelCmd = &cobra.Command{
	Use:   "cancel [id]",
	Short: "Cancel an execution",
	Long:  `Sets execution and all its pending/assigned/running jobs to canceled. Workers will stop claiming them.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runExecutionCancel,
}

func init() {
	executionListCmd.Flags().StringVar(&executionRunID, "run-id", "", "Filter by campaign run ID")
	executionListCmd.Flags().StringVar(&executionCampaignID, "campaign-id", "", "Filter by campaign ID")
	executionListCmd.Flags().StringVar(&executionStatus, "status", "", "Filter by status: planning|running|completed|failed|canceled")
	executionListCmd.Flags().IntVar(&executionLimit, "limit", 100, "Max items to return")
	executionListCmd.Flags().IntVar(&executionOffset, "offset", 0, "Offset for pagination")

	executionJobsCmd.Flags().IntVar(&executionJobLimit, "limit", 100, "Max jobs to return")
	executionJobsCmd.Flags().IntVar(&executionJobOffset, "offset", 0, "Offset for pagination")

	executionCmd.AddCommand(executionListCmd)
	executionCmd.AddCommand(executionGetCmd)
	executionCmd.AddCommand(executionJobsCmd)
	executionCmd.AddCommand(executionRequeueCmd)
	executionCmd.AddCommand(executionCancelCmd)
	rootCmd.AddCommand(executionCmd)
}

func runExecutionList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cl := client.New(getConfig())
	resp, err := cl.ExecutionList(ctx, executionRunID, executionCampaignID, executionStatus, executionLimit, executionOffset)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, resp)
	}
	output.PrintExecutionList(os.Stdout, resp)
	return nil
}

func runExecutionGet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cl := client.New(getConfig())
	e, err := cl.ExecutionGet(ctx, args[0])
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, e)
	}
	output.PrintExecution(os.Stdout, e)
	return nil
}

func runExecutionJobs(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cl := client.New(getConfig())
	resp, err := cl.ExecutionListJobs(ctx, args[0], executionJobLimit, executionJobOffset)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, resp)
	}
	output.PrintExecutionJobs(os.Stdout, resp.Items, resp.Total, resp.Limit, resp.Offset, "Execution jobs")
	return nil
}

func runExecutionRequeue(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cl := client.New(getConfig())
	requeued, err := cl.ExecutionRequeue(ctx, args[0])
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, map[string]int{"requeued": requeued})
	}
	fmt.Fprintf(os.Stdout, "  Requeued %d failed job(s).\n", requeued)
	return nil
}

func runExecutionCancel(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cl := client.New(getConfig())
	canceled, err := cl.ExecutionCancel(ctx, args[0])
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, map[string]interface{}{"canceled": canceled, "status": "canceled"})
	}
	fmt.Fprintf(os.Stdout, "  Canceled execution and %d job(s).\n", canceled)
	return nil
}
