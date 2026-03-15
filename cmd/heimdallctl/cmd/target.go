package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/client"
	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/output"
	"github.com/spf13/cobra"
)

var (
	targetIncludeInactive bool
	targetLimit           int
	targetOffset          int
	targetName            string
	targetDescription     string
	targetRules           []string
	targetDiffFrom         string
	targetDiffTo           string
)

var targetCmd = &cobra.Command{
	Use:   "target",
	Short: "Manage targets (definitions and materializations)",
	Long:  `Create, list, update, delete targets and trigger materialization. Targets are persistent scope definitions (include/exclude by country, ASN, prefix, world) that materialize to CIDR sets.`,
}

var targetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List targets (active by default)",
	RunE:  runTargetList,
}

var targetGetCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get target by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runTargetGet,
}

var targetCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a target",
	RunE:  runTargetCreate,
}

var targetUpdateCmd = &cobra.Command{
	Use:   "update [id]",
	Short: "Update target (full replacement of name, description, rules)",
	Long:  "If --name or --description are omitted, current values are kept. Rules are always replaced when --rule is used.",
	Args:  cobra.ExactArgs(1),
	RunE:  runTargetUpdate,
}

var targetDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Soft-delete target (idempotent)",
	Args:  cobra.ExactArgs(1),
	RunE:  runTargetDelete,
}

var targetMaterializeCmd = &cobra.Command{
	Use:   "materialize [id]",
	Short: "Trigger materialization for a target",
	Args:  cobra.ExactArgs(1),
	RunE:  runTargetMaterialize,
}

var targetMaterializationsCmd = &cobra.Command{
	Use:   "materializations [id]",
	Short: "List materializations (snapshots) for a target",
	Args:  cobra.ExactArgs(1),
	RunE:  runTargetMaterializations,
}

var targetPrefixesCmd = &cobra.Command{
	Use:   "prefixes [target-id] [materialization-id]",
	Short: "List prefixes for a materialization",
	Args:  cobra.ExactArgs(2),
	RunE:  runTargetPrefixes,
}

var targetDiffCmd = &cobra.Command{
	Use:   "diff [target-id]",
	Short: "Diff between two materializations",
	Long:  "Compares --from (base snapshot) to --to (new snapshot). 'Added' = prefixes in --to not in --from; 'Removed' = prefixes in --from not in --to. E.g. to see what was added when you added a country: --from <old-mid> --to <new-mid>.",
	Args:  cobra.ExactArgs(1),
	RunE:  runTargetDiff,
}

func init() {
	targetListCmd.Flags().BoolVar(&targetIncludeInactive, "include-inactive", false, "Include inactive targets")
	targetListCmd.Flags().IntVar(&targetLimit, "limit", 100, "Limit")
	targetListCmd.Flags().IntVar(&targetOffset, "offset", 0, "Offset")
	targetCmd.AddCommand(targetListCmd)

	targetCmd.AddCommand(targetGetCmd)

	targetCreateCmd.Flags().StringVar(&targetName, "name", "", "Target name (required)")
	targetCreateCmd.Flags().StringVar(&targetDescription, "description", "", "Description")
	targetCreateCmd.Flags().StringArrayVar(&targetRules, "rule", nil, "Rule: kind,selector_type,selector_value (e.g. include,country,ES). Repeat --rule for multiple.")
	targetCreateCmd.MarkFlagRequired("name")
	targetCmd.AddCommand(targetCreateCmd)

	targetUpdateCmd.Flags().StringVar(&targetName, "name", "", "Target name (optional; keeps current if omitted)")
	targetUpdateCmd.Flags().StringVar(&targetDescription, "description", "", "Description (optional; keeps current if omitted)")
	targetUpdateCmd.Flags().StringArrayVar(&targetRules, "rule", nil, "Rules (replaces all). Format: kind,selector_type,selector_value (e.g. include,country,ES). Repeat --rule for multiple.")
	targetCmd.AddCommand(targetUpdateCmd)

	targetCmd.AddCommand(targetDeleteCmd)
	targetCmd.AddCommand(targetMaterializeCmd)

	targetMaterializationsCmd.Flags().IntVar(&targetLimit, "limit", 100, "Limit")
	targetMaterializationsCmd.Flags().IntVar(&targetOffset, "offset", 0, "Offset")
	targetCmd.AddCommand(targetMaterializationsCmd)

	targetPrefixesCmd.Flags().IntVar(&targetLimit, "limit", 100, "Limit")
	targetPrefixesCmd.Flags().IntVar(&targetOffset, "offset", 0, "Offset")
	targetCmd.AddCommand(targetPrefixesCmd)

	targetDiffCmd.Flags().StringVar(&targetDiffFrom, "from", "", "From materialization ID (required)")
	targetDiffCmd.Flags().StringVar(&targetDiffTo, "to", "", "To materialization ID (required)")
	targetDiffCmd.MarkFlagRequired("from")
	targetDiffCmd.MarkFlagRequired("to")
	targetCmd.AddCommand(targetDiffCmd)
}

func parseRules(ruleStrs []string) []client.TargetRuleInput {
	var rules []client.TargetRuleInput
	for i, s := range ruleStrs {
		parts := strings.SplitN(s, ",", 3)
		if len(parts) < 3 {
			continue
		}
		kind := strings.TrimSpace(parts[0])
		selType := strings.TrimSpace(parts[1])
		selVal := ""
		if len(parts) > 2 {
			selVal = strings.TrimSpace(parts[2])
		}
		rules = append(rules, client.TargetRuleInput{
			Kind:          kind,
			SelectorType:  selType,
			SelectorValue: selVal,
			RuleOrder:     i,
		})
	}
	return rules
}

func runTargetList(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.TargetList(ctx, targetIncludeInactive, targetLimit, targetOffset)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintTargetList(os.Stdout, r)
	return nil
}

func runTargetGet(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.TargetGet(ctx, args[0])
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintTargetGet(os.Stdout, r)
	return nil
}

func runTargetCreate(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	in := &client.TargetCreateInput{
		Name:        targetName,
		Description: targetDescription,
		Rules:       parseRules(targetRules),
	}
	r, err := cl.TargetCreate(ctx, in)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintTargetGet(os.Stdout, r)
	return nil
}

func runTargetUpdate(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	cur, err := cl.TargetGet(ctx, args[0])
	if err != nil {
		return err
	}
	name := targetName
	if name == "" {
		name = cur.Name
	}
	description := targetDescription
	if description == "" {
		description = cur.Description
	}
	rules := parseRules(targetRules)
	if len(rules) == 0 {
		for i, r := range cur.Rules {
			rules = append(rules, client.TargetRuleInput{
				Kind:          r.Kind,
				SelectorType:  r.SelectorType,
				SelectorValue: r.SelectorValue,
				AddressFamily: r.AddressFamily,
				RuleOrder:     i,
			})
		}
	}
	in := &client.TargetCreateInput{
		Name:        name,
		Description: description,
		Rules:       rules,
	}
	r, err := cl.TargetUpdate(ctx, args[0], in)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintTargetGet(os.Stdout, r)
	return nil
}

func runTargetDelete(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	if err := cl.TargetDelete(ctx, args[0]); err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, map[string]string{"status": "deleted", "id": args[0]})
	}
	os.Stdout.WriteString("Target " + args[0] + " soft-deleted.\n")
	return nil
}

func runTargetMaterialize(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.TargetMaterialize(ctx, args[0])
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintTargetMaterialize(os.Stdout, r)
	return nil
}

func runTargetMaterializations(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.TargetMaterializations(ctx, args[0], targetLimit, targetOffset)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintTargetMaterializations(os.Stdout, r)
	return nil
}

func runTargetPrefixes(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.TargetPrefixes(ctx, args[0], args[1], targetLimit, targetOffset)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintTargetPrefixes(os.Stdout, r)
	return nil
}

func runTargetDiff(cmd *cobra.Command, args []string) error {
	cl := client.New(getConfig())
	ctx := context.Background()
	r, err := cl.TargetDiff(ctx, args[0], targetDiffFrom, targetDiffTo)
	if err != nil {
		return err
	}
	if isJSON() {
		return output.PrintJSON(os.Stdout, r)
	}
	output.PrintTargetDiff(os.Stdout, r)
	return nil
}
