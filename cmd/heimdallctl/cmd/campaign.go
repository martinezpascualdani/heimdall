package cmd

import (
	"context"
	"encoding/json"
	"os"

	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/client"
	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/output"
	"github.com/spf13/cobra"
)

var (
	campaignIncludeInactive bool
	campaignLimit           int
	campaignOffset          int
	campaignName            string
	campaignDescription     string
	campaignTargetID        string
	campaignScanProfileID   string
	campaignScheduleType    string
	campaignMaterialization string
	campaignConcurrency     string
	campaignNextRunAt       string
	campaignScheduleConfig  string

	scanProfileName        string
	scanProfileSlug        string
	scanProfileDescription string
	scanProfileConfig      string
)

var campaignCmd = &cobra.Command{
	Use:   "campaign",
	Short: "Manage campaigns and runs (campaign-service)",
	Long:  `Create, list, update, delete campaigns; launch runs; list and cancel runs.`,
}

var campaignListCmd = &cobra.Command{
	Use:   "list",
	Short: "List campaigns (active by default)",
	RunE:  runCampaignList,
}

var campaignGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get campaign by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runCampaignGet,
}

var campaignCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a campaign",
	RunE:  runCampaignCreate,
}

var campaignUpdateCmd = &cobra.Command{
	Use:   "update [id]",
	Short: "Update campaign (full replacement)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCampaignUpdate,
}

var campaignDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Soft-delete campaign",
	Args:  cobra.ExactArgs(1),
	RunE:  runCampaignDelete,
}

var campaignLaunchCmd = &cobra.Command{
	Use:   "launch [id]",
	Short: "Launch campaign (create run and dispatch)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCampaignLaunch,
}

var campaignRunsCmd = &cobra.Command{
	Use:   "runs [campaign-id]",
	Short: "List runs for a campaign",
	Args:  cobra.ExactArgs(1),
	RunE:  runCampaignRuns,
}

var campaignRunGetCmd = &cobra.Command{
	Use:   "get [run-id]",
	Short: "Get run by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runRunGet,
}

var campaignRunCancelCmd = &cobra.Command{
	Use:   "cancel [run-id]",
	Short: "Cancel a run",
	Args:  cobra.ExactArgs(1),
	RunE:  runRunCancel,
}

var campaignRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Get or cancel a run",
}

// --- scan-profile ---

var scanProfileCmd = &cobra.Command{
	Use:   "scan-profile",
	Short: "Manage scan profiles (campaign-service)",
	Long:  `Create, list, update, delete scan profiles.`,
}

var scanProfileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List scan profiles",
	RunE:  runScanProfileList,
}

var scanProfileGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get scan profile by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runScanProfileGet,
}

var scanProfileCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a scan profile",
	RunE:  runScanProfileCreate,
}

var scanProfileUpdateCmd = &cobra.Command{
	Use:   "update [id]",
	Short: "Update scan profile (full replacement)",
	Args:  cobra.ExactArgs(1),
	RunE:  runScanProfileUpdate,
}

var scanProfileDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete scan profile (rejected if in use)",
	Args:  cobra.ExactArgs(1),
	RunE:  runScanProfileDelete,
}

func init() {
	// campaign flags
	campaignListCmd.Flags().BoolVar(&campaignIncludeInactive, "include-inactive", false, "Include inactive campaigns")
	campaignListCmd.Flags().IntVar(&campaignLimit, "limit", 100, "Limit")
	campaignListCmd.Flags().IntVar(&campaignOffset, "offset", 0, "Offset")
	campaignCmd.AddCommand(campaignListCmd)
	campaignCmd.AddCommand(campaignGetCmd)

	campaignCreateCmd.Flags().StringVar(&campaignName, "name", "", "Campaign name (required)")
	campaignCreateCmd.Flags().StringVar(&campaignDescription, "description", "", "Description")
	campaignCreateCmd.Flags().StringVar(&campaignTargetID, "target-id", "", "Target ID (required)")
	campaignCreateCmd.Flags().StringVar(&campaignScanProfileID, "scan-profile-id", "", "Scan profile ID (required)")
	campaignCreateCmd.Flags().StringVar(&campaignScheduleType, "schedule-type", "manual", "Schedule: manual | once | interval")
	campaignCreateCmd.Flags().StringVar(&campaignMaterialization, "materialization-policy", "use_latest", "use_latest | rematerialize")
	campaignCreateCmd.Flags().StringVar(&campaignConcurrency, "concurrency-policy", "allow", "allow | forbid_if_active")
	campaignCreateCmd.Flags().StringVar(&campaignNextRunAt, "next-run-at", "", "Next run time (optional)")
	campaignCreateCmd.Flags().StringVar(&campaignScheduleConfig, "schedule-config", "", "JSON for interval config (e.g. {\"interval_seconds\":3600})")
	campaignCreateCmd.MarkFlagRequired("name")
	campaignCreateCmd.MarkFlagRequired("target-id")
	campaignCreateCmd.MarkFlagRequired("scan-profile-id")
	campaignCmd.AddCommand(campaignCreateCmd)

	campaignUpdateCmd.Flags().StringVar(&campaignName, "name", "", "Campaign name")
	campaignUpdateCmd.Flags().StringVar(&campaignDescription, "description", "", "Description")
	campaignUpdateCmd.Flags().StringVar(&campaignTargetID, "target-id", "", "Target ID")
	campaignUpdateCmd.Flags().StringVar(&campaignScanProfileID, "scan-profile-id", "", "Scan profile ID")
	campaignUpdateCmd.Flags().StringVar(&campaignScheduleType, "schedule-type", "", "manual | once | interval")
	campaignUpdateCmd.Flags().StringVar(&campaignMaterialization, "materialization-policy", "", "use_latest | rematerialize")
	campaignUpdateCmd.Flags().StringVar(&campaignConcurrency, "concurrency-policy", "", "allow | forbid_if_active")
	campaignUpdateCmd.Flags().StringVar(&campaignNextRunAt, "next-run-at", "", "Next run time")
	campaignUpdateCmd.Flags().StringVar(&campaignScheduleConfig, "schedule-config", "", "JSON for interval config")
	campaignCmd.AddCommand(campaignUpdateCmd)

	campaignCmd.AddCommand(campaignDeleteCmd)
	campaignCmd.AddCommand(campaignLaunchCmd)

	campaignRunsCmd.Flags().IntVar(&campaignLimit, "limit", 100, "Limit")
	campaignRunsCmd.Flags().IntVar(&campaignOffset, "offset", 0, "Offset")
	campaignCmd.AddCommand(campaignRunsCmd)

	campaignRunCmd.AddCommand(campaignRunGetCmd)
	campaignRunCmd.AddCommand(campaignRunCancelCmd)
	campaignCmd.AddCommand(campaignRunCmd)

	// scan-profile flags
	scanProfileListCmd.Flags().IntVar(&campaignLimit, "limit", 100, "Limit")
	scanProfileListCmd.Flags().IntVar(&campaignOffset, "offset", 0, "Offset")
	scanProfileCmd.AddCommand(scanProfileListCmd)
	scanProfileCmd.AddCommand(scanProfileGetCmd)

	scanProfileCreateCmd.Flags().StringVar(&scanProfileName, "name", "", "Name (required)")
	scanProfileCreateCmd.Flags().StringVar(&scanProfileSlug, "slug", "", "Slug (required)")
	scanProfileCreateCmd.Flags().StringVar(&scanProfileDescription, "description", "", "Description")
	scanProfileCreateCmd.Flags().StringVar(&scanProfileConfig, "config", "", "JSON config object (optional)")
	scanProfileCreateCmd.MarkFlagRequired("name")
	scanProfileCreateCmd.MarkFlagRequired("slug")
	scanProfileCmd.AddCommand(scanProfileCreateCmd)

	scanProfileUpdateCmd.Flags().StringVar(&scanProfileName, "name", "", "Name")
	scanProfileUpdateCmd.Flags().StringVar(&scanProfileSlug, "slug", "", "Slug")
	scanProfileUpdateCmd.Flags().StringVar(&scanProfileDescription, "description", "", "Description")
	scanProfileUpdateCmd.Flags().StringVar(&scanProfileConfig, "config", "", "JSON config object")
	scanProfileCmd.AddCommand(scanProfileUpdateCmd)
	scanProfileCmd.AddCommand(scanProfileDeleteCmd)

	rootCmd.AddCommand(scanProfileCmd)
}

func parseScheduleConfig(s string) (interface{}, error) {
	if s == "" {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, err
	}
	return v, nil
}

func parseConfig(s string) (interface{}, error) {
	if s == "" {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, err
	}
	return v, nil
}

func runCampaignList(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.CampaignList(ctx, campaignIncludeInactive, campaignLimit, campaignOffset)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCampaignList(os.Stdout, r)
	return nil
}

func runCampaignGet(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.CampaignGet(ctx, args[0])
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCampaignGet(os.Stdout, r)
	return nil
}

func runCampaignCreate(cmd *cobra.Command, args []string) error {
	cfg, err := parseScheduleConfig(campaignScheduleConfig)
	if err != nil {
		return err
	}
	cl := client.New(getConfig())
	ctx := context.Background()
	in := &client.CampaignCreateInput{
		Name:                  campaignName,
		Description:           campaignDescription,
		TargetID:              campaignTargetID,
		ScanProfileID:         campaignScanProfileID,
		ScheduleType:          campaignScheduleType,
		ScheduleConfig:        cfg,
		MaterializationPolicy: campaignMaterialization,
		NextRunAt:             campaignNextRunAt,
		ConcurrencyPolicy:     campaignConcurrency,
	}
	r, err := cl.CampaignCreate(ctx, in)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCampaignGet(os.Stdout, r)
	return nil
}

func runCampaignUpdate(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	cur, err := cl.CampaignGet(ctx, args[0])
	if err != nil {
		return err
	}
	name := campaignName
	if name == "" {
		name = cur.Name
	}
	desc := campaignDescription
	if desc == "" {
		desc = cur.Description
	}
	targetID := campaignTargetID
	if targetID == "" {
		targetID = cur.TargetID
	}
	scanProfileID := campaignScanProfileID
	if scanProfileID == "" {
		scanProfileID = cur.ScanProfileID
	}
	scheduleType := campaignScheduleType
	if scheduleType == "" {
		scheduleType = cur.ScheduleType
	}
	matPol := campaignMaterialization
	if matPol == "" {
		matPol = cur.MaterializationPolicy
	}
	concPol := campaignConcurrency
	if concPol == "" {
		concPol = cur.ConcurrencyPolicy
	}
	nextRun := campaignNextRunAt
	if nextRun == "" {
		nextRun = cur.NextRunAt
	}
	cfg := cur.ScheduleConfig
	if campaignScheduleConfig != "" {
		parsed, err := parseScheduleConfig(campaignScheduleConfig)
		if err != nil {
			return err
		}
		cfg = parsed
	}
	in := &client.CampaignCreateInput{
		Name:                  name,
		Description:           desc,
		TargetID:              targetID,
		ScanProfileID:         scanProfileID,
		ScheduleType:          scheduleType,
		ScheduleConfig:        cfg,
		MaterializationPolicy: matPol,
		NextRunAt:             nextRun,
		ConcurrencyPolicy:     concPol,
	}
	r, err := cl.CampaignUpdate(ctx, args[0], in)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCampaignGet(os.Stdout, r)
	return nil
}

func runCampaignDelete(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	if err := cl.CampaignDelete(ctx, args[0]); err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, map[string]string{"status": "deleted", "id": args[0]})
	}
	os.Stdout.WriteString("Campaign " + args[0] + " soft-deleted.\n")
	return nil
}

func runCampaignLaunch(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.CampaignLaunch(ctx, args[0])
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCampaignRunGet(os.Stdout, r)
	return nil
}

func runCampaignRuns(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.CampaignRunList(ctx, args[0], campaignLimit, campaignOffset)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCampaignRunList(os.Stdout, r)
	return nil
}

func runRunGet(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.RunGet(ctx, args[0])
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCampaignRunGet(os.Stdout, r)
	return nil
}

func runRunCancel(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.RunCancel(ctx, args[0])
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintCampaignRunGet(os.Stdout, r)
	return nil
}

func runScanProfileList(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.ScanProfileList(ctx, campaignLimit, campaignOffset)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintScanProfileList(os.Stdout, r)
	return nil
}

func runScanProfileGet(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.ScanProfileGet(ctx, args[0])
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintScanProfileGet(os.Stdout, r)
	return nil
}

func runScanProfileCreate(cmd *cobra.Command, args []string) error {
	cfg, err := parseConfig(scanProfileConfig)
	if err != nil {
		return err
	}
	cl := client.New(getConfig())
	ctx := context.Background()
	in := &client.ScanProfileCreateInput{
		Name:        scanProfileName,
		Slug:        scanProfileSlug,
		Description: scanProfileDescription,
		Config:      cfg,
	}
	r, err := cl.ScanProfileCreate(ctx, in)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintScanProfileGet(os.Stdout, r)
	return nil
}

func runScanProfileUpdate(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	cur, err := cl.ScanProfileGet(ctx, args[0])
	if err != nil {
		return err
	}
	name := scanProfileName
	if name == "" {
		name = cur.Name
	}
	slug := scanProfileSlug
	if slug == "" {
		slug = cur.Slug
	}
	desc := scanProfileDescription
	if desc == "" {
		desc = cur.Description
	}
	cfg := cur.Config
	if scanProfileConfig != "" {
		parsed, err := parseConfig(scanProfileConfig)
		if err != nil {
			return err
		}
		cfg = parsed
	}
	in := &client.ScanProfileCreateInput{
		Name:        name,
		Slug:        slug,
		Description: desc,
		Config:      cfg,
	}
	r, err := cl.ScanProfileUpdate(ctx, args[0], in)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintScanProfileGet(os.Stdout, r)
	return nil
}

func runScanProfileDelete(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	if err := cl.ScanProfileDelete(ctx, args[0]); err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, map[string]string{"status": "deleted", "id": args[0]})
	}
	os.Stdout.WriteString("Scan profile " + args[0] + " deleted.\n")
	return nil
}
