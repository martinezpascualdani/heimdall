package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/client"
)

const labelWidth = 18

// kv prints one key-value line with aligned label (for human-readable blocks).
func kv(w io.Writer, label, value string) {
	fmt.Fprintf(w, "  %-*s  %s\n", labelWidth, label+":", value)
}

// kvInt prints a key-value line for an integer.
func kvInt(w io.Writer, label string, value int64) {
	fmt.Fprintf(w, "  %-*s  %d\n", labelWidth, label+":", value)
}

// section prints a section title with a simple visual separator.
func section(w io.Writer, title string) {
	fmt.Fprintf(w, "  %s\n  %s\n\n", title, strings.Repeat("─", len(title)+2))
}

// PrintJSON encodes v as JSON to w.
func PrintJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// PrintDatasetList writes a table of datasets to w.
func PrintDatasetList(w io.Writer, list *client.DatasetListResponse) {
	if list == nil || len(list.Datasets) == 0 {
		fmt.Fprintln(w, "  No datasets.")
		return
	}
	section(w, "Datasets")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  ID\tSOURCE\tTYPE\tSTATE\tRECORDS\tCREATED_AT")
	fmt.Fprintln(tw, "  ──\t──────\t────\t──────\t───────\t──────────")
	for _, d := range list.Datasets {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%d\t%s\n",
			trunc(d.ID, 8),
			d.Source,
			d.SourceType,
			d.State,
			d.RecordCount,
			d.CreatedAt,
		)
	}
	tw.Flush()
	fmt.Fprintln(w)
}

// PrintScopeSync writes scope sync results as a table.
func PrintScopeSync(w io.Writer, r *client.ScopeSyncResponse) {
	if r == nil || len(r.Results) == 0 {
		fmt.Fprintln(w, "  No results.")
		return
	}
	section(w, "Scope sync")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  REGISTRY\tSTATUS\tBLOCKS\tASNS\tDURATION\tERROR")
	fmt.Fprintln(tw, "  ────────\t──────\t──────\t────\t────────\t─────")
	for _, x := range r.Results {
		errStr := x.Error
		if errStr == "" {
			errStr = "—"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%d\t%d\t%d ms\t%s\n",
			x.Registry, x.Status, x.BlocksPersisted, x.ASNsPersisted, x.DurationMs, errStr)
	}
	tw.Flush()
	fmt.Fprintln(w)
}

// PrintRoutingSync writes routing sync results as a table.
func PrintRoutingSync(w io.Writer, r *client.RoutingSyncResponse) {
	if r == nil || len(r.Results) == 0 {
		fmt.Fprintln(w, "  No results.")
		return
	}
	section(w, "Routing sync")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  SOURCE\tSTATUS\tROWS\tDURATION\tERROR")
	fmt.Fprintln(tw, "  ──────\t──────\t────\t────────\t─────")
	for _, x := range r.Results {
		errStr := x.Error
		if errStr == "" {
			errStr = "—"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%d\t%d ms\t%s\n",
			x.Source, x.Status, x.RowsPersisted, x.DurationMs, errStr)
	}
	tw.Flush()
	fmt.Fprintln(w)
}

// PrintIPResolve writes scope by-ip result (key-value block).
func PrintIPResolve(w io.Writer, r *client.IPResolveResponse) {
	if r == nil {
		return
	}
	section(w, "Scope · IP → Country")
	kv(w, "IP", r.IP)
	kv(w, "Country", r.ScopeValue)
	kv(w, "Scope type", r.ScopeType)
	kv(w, "Dataset ID", r.DatasetID)
	if r.Registry != "" {
		kv(w, "Registry", r.Registry)
	}
	if r.Serial != 0 {
		kvInt(w, "Serial", r.Serial)
	}
	fmt.Fprintln(w)
}

// PrintRoutingByIP writes routing by-ip result (key-value block).
func PrintRoutingByIP(w io.Writer, r *client.RoutingByIPResponse) {
	if r == nil {
		return
	}
	section(w, "Routing · IP → ASN")
	kv(w, "IP", r.IP)
	kv(w, "Matched prefix", r.MatchedPrefix)
	kv(w, "ASN (raw)", r.ASNRaw)
	if r.PrimaryASN != nil {
		kvInt(w, "Primary ASN", *r.PrimaryASN)
	}
	kv(w, "Type", r.ASNType)
	if r.ASName != "" {
		kv(w, "AS name", r.ASName)
	}
	if r.OrgName != "" {
		kv(w, "Organization", r.OrgName)
	}
	fmt.Fprintln(w)
}

// PrintCountrySummary writes scope country summary.
func PrintCountrySummary(w io.Writer, r *client.CountrySummaryResponse) {
	if r == nil {
		return
	}
	section(w, "Country summary · "+r.ScopeValue)
	kv(w, "Country", r.ScopeValue)
	kvInt(w, "IPv4 blocks", r.IPv4BlockCount)
	kvInt(w, "IPv6 blocks", r.IPv6BlockCount)
	kvInt(w, "Total blocks", r.Total)
	if len(r.DatasetsUsed) > 0 {
		fmt.Fprintf(w, "\n  %-*s  ", labelWidth, "Datasets used:")
		for i, d := range r.DatasetsUsed {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			fmt.Fprintf(w, "%s (%s)", trunc(d.DatasetID, 8), d.Registry)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}

// PrintRoutingASNMeta writes ASN metadata (routing).
func PrintRoutingASNMeta(w io.Writer, r *client.RoutingASNMetaResponse) {
	if r == nil {
		return
	}
	section(w, "ASN metadata · "+fmt.Sprintf("%d", r.ASN))
	kvInt(w, "ASN", int64(r.ASN))
	if r.ASName != "" {
		kv(w, "Name", r.ASName)
	}
	if r.OrgName != "" {
		kv(w, "Organization", r.OrgName)
	}
	if r.Source != "" {
		kv(w, "Source", r.Source)
	}
	fmt.Fprintln(w)
}

// PrintASNPrefixes writes routing ASN prefixes list.
func PrintASNPrefixes(w io.Writer, r *client.RoutingASNPrefixesResponse) {
	if r == nil {
		return
	}
	section(w, fmt.Sprintf("ASN %d · Prefixes (%d total)", r.ASN, r.Total))
	if len(r.Items) == 0 {
		fmt.Fprintln(w, "  No prefixes.")
		fmt.Fprintln(w)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  PREFIX\tLEN\tASN_RAW\tTYPE")
	fmt.Fprintln(tw, "  ──────\t───\t───────\t────")
	for _, p := range r.Items {
		fmt.Fprintf(tw, "  %s\t%d\t%s\t%s\n", p.Prefix, p.PrefixLength, p.ASNRaw, p.ASNType)
	}
	tw.Flush()
	if r.HasMore {
		fmt.Fprintf(w, "\n  … more (offset %d)\n", r.Offset)
	}
	fmt.Fprintln(w)
}

// PrintStatus writes health status of dataset, scope, routing, target, campaign, and execution services.
func PrintStatus(w io.Writer, dataset, scope, routing, target, campaign, execution client.HealthResult) {
	section(w, "Service status")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  SERVICE\tSTATUS\tDETAIL")
	fmt.Fprintln(tw, "  ───────\t──────\t──────")
	statusRow("dataset", dataset, tw)
	statusRow("scope", scope, tw)
	statusRow("routing", routing, tw)
	statusRow("target", target, tw)
	statusRow("campaign", campaign, tw)
	statusRow("execution", execution, tw)
	tw.Flush()
	fmt.Fprintln(w)
}

func statusRow(name string, r client.HealthResult, w *tabwriter.Writer) {
	if r.OK {
		fmt.Fprintf(w, "  %s\t  ✓ ok\t  —\n", name)
	} else {
		fmt.Fprintf(w, "  %s\t  ✗ fail\t  %s\n", name, r.Error)
	}
}

// PrintFetchResult writes dataset fetch result (single or all).
func PrintFetchResult(w io.Writer, v interface{}) {
	switch r := v.(type) {
	case *client.FetchResultAll:
		if r == nil || len(r.Results) == 0 {
			fmt.Fprintln(w, "  No results.")
			return
		}
		section(w, "Dataset fetch")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  REGISTRY\tSTATUS\tDATASET_ID\tSTATE\tERROR")
		fmt.Fprintln(tw, "  ────────\t──────\t──────────\t──────\t─────")
		for _, x := range r.Results {
			errStr := x.Error
			if errStr == "" {
				errStr = "—"
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", x.Registry, x.Status, trunc(x.DatasetID, 8), x.State, errStr)
		}
		tw.Flush()
		fmt.Fprintln(w)
	case *client.FetchResultSingle:
		if r == nil {
			return
		}
		section(w, "Dataset fetch")
		kv(w, "Status", r.Status)
		kv(w, "Dataset ID", r.DatasetID)
		if r.Registry != "" {
			kv(w, "Registry", r.Registry)
		}
		if r.State != "" {
			kv(w, "State", r.State)
		}
		if r.Error != "" {
			kv(w, "Error", r.Error)
		}
		fmt.Fprintln(w)
	default:
		fmt.Fprintf(w, "%v\n", v)
	}
}

// PrintDatasetGet writes a single dataset version.
func PrintDatasetGet(w io.Writer, d *client.DatasetVersion) {
	if d == nil {
		return
	}
	section(w, "Dataset · "+trunc(d.ID, 8))
	kv(w, "ID", d.ID)
	kv(w, "Source", d.Source)
	kv(w, "Source type", d.SourceType)
	kv(w, "State", d.State)
	kvInt(w, "Record count", d.RecordCount)
	if d.CreatedAt != "" {
		kv(w, "Created", d.CreatedAt)
	}
	fmt.Fprintln(w)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// PrintCountryBlocks writes scope country blocks (table).
func PrintCountryBlocks(w io.Writer, r *client.CountryBlocksResponse) {
	if r == nil {
		return
	}
	section(w, fmt.Sprintf("Country %s · Blocks (%d of %d)", r.ScopeValue, r.Count, r.Total))
	if len(r.Items) == 0 {
		fmt.Fprintln(w, "  No blocks.")
		fmt.Fprintln(w)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  START\tEND\tCOUNT\tSTATUS")
	fmt.Fprintln(tw, "  ─────\t───\t─────\t──────")
	for _, b := range r.Items {
		fmt.Fprintf(tw, "  %s\t%s\t%d\t%s\n", b.StartValue, b.EndValue, b.Count, b.Status)
	}
	tw.Flush()
	if r.HasMore {
		fmt.Fprintf(w, "\n  … more (offset %d)\n", r.Offset)
	}
	fmt.Fprintln(w)
}

// PrintCountryASNs writes scope country ASNs (table).
func PrintCountryASNs(w io.Writer, r *client.CountryASNsResponse) {
	if r == nil {
		return
	}
	section(w, fmt.Sprintf("Country %s · ASN ranges (%d of %d)", r.ScopeValue, r.Count, r.Total))
	if len(r.Items) == 0 {
		fmt.Fprintln(w, "  No ASN ranges.")
		fmt.Fprintln(w)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  ASN_START\tASN_END\tREGISTRY\tDATE")
	fmt.Fprintln(tw, "  ─────────\t───────\t────────\t────")
	for _, a := range r.Items {
		fmt.Fprintf(tw, "  %d\t%d\t%s\t%s\n", a.ASNStart, a.ASNEnd, a.Registry, a.Date)
	}
	tw.Flush()
	if r.HasMore {
		fmt.Fprintf(w, "\n  … more (offset %d)\n", r.Offset)
	}
	fmt.Fprintln(w)
}

// PrintCountryASNSummary writes scope country asn-summary.
func PrintCountryASNSummary(w io.Writer, r *client.CountryASNSummaryResponse) {
	if r == nil {
		return
	}
	section(w, "Country "+r.ScopeValue+" · ASN summary")
	kv(w, "Country", r.ScopeValue)
	kvInt(w, "ASN range count", int64(r.ASNRangeCount))
	kvInt(w, "ASN total count", r.ASNTotalCount)
	fmt.Fprintln(w)
}

// PrintCountryDatasets writes scope country datasets list.
func PrintCountryDatasets(w io.Writer, r *client.CountryDatasetsResponse) {
	if r == nil {
		return
	}
	section(w, "Country "+r.ScopeValue+" · Datasets with blocks")
	if len(r.Datasets) == 0 {
		fmt.Fprintln(w, "  No datasets.")
		fmt.Fprintln(w)
		return
	}
	for _, d := range r.Datasets {
		fmt.Fprintf(w, "  · %s  (%s)\n", d.DatasetID, d.Registry)
	}
	fmt.Fprintln(w)
}

// Format is output format: "table" or "json".
func Format(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "json" {
		return "json"
	}
	return "table"
}

// OutputFormat returns the format from a flag value (e.g. -o json).
func OutputFormat(flag string) string {
	return Format(flag)
}

// PrintTargetList writes targets table.
func PrintTargetList(w io.Writer, r *client.TargetListResponse) {
	if r == nil || len(r.Items) == 0 {
		fmt.Fprintln(w, "  No targets.")
		return
	}
	section(w, "Targets")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  ID\tNAME\tACTIVE\tRULES\tCREATED")
	fmt.Fprintln(tw, "  ──\t────\t──────\t─────\t───────")
	for _, t := range r.Items {
		active := "no"
		if t.Active {
			active = "yes"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%d\t%s\n", t.ID, t.Name, active, len(t.Rules), t.CreatedAt)
	}
	tw.Flush()
	fmt.Fprintln(w)
}

// PrintTargetGet writes a single target with rules.
func PrintTargetGet(w io.Writer, t *client.TargetResponse) {
	if t == nil {
		return
	}
	section(w, "Target · "+t.Name)
	kv(w, "ID", t.ID)
	kv(w, "Name", t.Name)
	kv(w, "Description", t.Description)
	kv(w, "Active", fmt.Sprintf("%v", t.Active))
	kv(w, "Created", t.CreatedAt)
	kv(w, "Updated", t.UpdatedAt)
	if len(t.Rules) > 0 {
		fmt.Fprintln(w, "  Rules:")
		for _, r := range t.Rules {
			fmt.Fprintf(w, "    · %s %s %s\n", r.Kind, r.SelectorType, r.SelectorValue)
		}
	}
	fmt.Fprintln(w)
}

// PrintTargetMaterialize writes materialize result.
func PrintTargetMaterialize(w io.Writer, r *client.TargetMaterializeResponse) {
	if r == nil {
		return
	}
	section(w, "Materialization")
	kv(w, "Materialization ID", r.MaterializationID)
	kv(w, "Status", r.Status)
	kvInt(w, "Total prefixes", int64(r.TotalPrefixCount))
	kv(w, "Materialized at", r.MaterializedAt)
	fmt.Fprintln(w)
}

// PrintTargetMaterializations writes materializations list.
func PrintTargetMaterializations(w io.Writer, r *client.TargetMaterializationsResponse) {
	if r == nil || len(r.Items) == 0 {
		fmt.Fprintln(w, "  No materializations.")
		return
	}
	section(w, "Materializations")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  ID\tMATERIALIZED_AT\tPREFIXES\tSTATUS")
	fmt.Fprintln(tw, "  ──\t───────────────\t────────\t──────")
	for _, m := range r.Items {
		fmt.Fprintf(tw, "  %s\t%s\t%d\t%s\n", m.ID, m.MaterializedAt, m.TotalPrefixCount, m.Status)
	}
	tw.Flush()
	fmt.Fprintln(w)
}

// PrintTargetPrefixes writes prefixes list.
func PrintTargetPrefixes(w io.Writer, r *client.TargetPrefixesResponse) {
	if r == nil {
		return
	}
	section(w, fmt.Sprintf("Prefixes (%d of %d)", r.Count, r.Total))
	for _, p := range r.Items {
		fmt.Fprintf(w, "  %s\n", p)
	}
	if r.HasMore {
		fmt.Fprintf(w, "\n  … more (offset %d)\n", r.Offset)
	}
	fmt.Fprintln(w)
}

// PrintTargetDiff writes diff result.
func PrintTargetDiff(w io.Writer, r *client.TargetDiffResponse) {
	if r == nil {
		return
	}
	section(w, "Diff")
	kv(w, "From", r.FromMaterializationID+" @ "+r.FromMaterializedAt)
	kv(w, "To", r.ToMaterializationID+" @ "+r.ToMaterializedAt)
	kvInt(w, "Added", int64(r.AddedCount))
	kvInt(w, "Removed", int64(r.RemovedCount))
	if len(r.Added) > 0 {
		fmt.Fprintln(w, "  Added prefixes:")
		for _, p := range r.Added {
			fmt.Fprintf(w, "    + %s\n", p)
		}
	}
	if len(r.Removed) > 0 {
		fmt.Fprintln(w, "  Removed prefixes:")
		for _, p := range r.Removed {
			fmt.Fprintf(w, "    - %s\n", p)
		}
	}
	fmt.Fprintln(w)
}

// PrintScanProfileList writes scan profiles table.
func PrintScanProfileList(w io.Writer, r *client.ScanProfileListResponse) {
	if r == nil || len(r.Items) == 0 {
		fmt.Fprintln(w, "  No scan profiles.")
		return
	}
	section(w, "Scan profiles")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  ID\tNAME\tSLUG\tCREATED_AT")
	fmt.Fprintln(tw, "  ──\t────\t────\t──────────")
	for _, p := range r.Items {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", p.ID, p.Name, p.Slug, p.CreatedAt)
	}
	tw.Flush()
	if r.HasMore {
		fmt.Fprintf(w, "\n  … more (total %d)\n", r.Total)
	}
	fmt.Fprintln(w)
}

// PrintScanProfileGet writes a single scan profile.
func PrintScanProfileGet(w io.Writer, p *client.ScanProfileResponse) {
	if p == nil {
		return
	}
	section(w, "Scan profile · "+p.Slug)
	kv(w, "ID", p.ID)
	kv(w, "Name", p.Name)
	kv(w, "Slug", p.Slug)
	kv(w, "Description", p.Description)
	kv(w, "Created", p.CreatedAt)
	kv(w, "Updated", p.UpdatedAt)
	fmt.Fprintln(w)
}

// PrintCampaignList writes campaigns table.
func PrintCampaignList(w io.Writer, r *client.CampaignListResponse) {
	if r == nil || len(r.Items) == 0 {
		fmt.Fprintln(w, "  No campaigns.")
		return
	}
	section(w, "Campaigns")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  ID\tNAME\tSCHEDULE\tMAT_POLICY\tACTIVE\tCREATED_AT")
	fmt.Fprintln(tw, "  ──\t────\t────────\t──────────\t──────\t──────────")
	for _, c := range r.Items {
		active := "no"
		if c.Active {
			active = "yes"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\n", c.ID, c.Name, c.ScheduleType, c.MaterializationPolicy, active, c.CreatedAt)
	}
	tw.Flush()
	if r.HasMore {
		fmt.Fprintf(w, "\n  … more (total %d)\n", r.Total)
	}
	fmt.Fprintln(w)
}

// PrintCampaignGet writes a single campaign.
func PrintCampaignGet(w io.Writer, c *client.CampaignResponse) {
	if c == nil {
		return
	}
	section(w, "Campaign · "+c.Name)
	kv(w, "ID", c.ID)
	kv(w, "Name", c.Name)
	kv(w, "Description", c.Description)
	kv(w, "Target ID", c.TargetID)
	kv(w, "Scan profile ID", c.ScanProfileID)
	kv(w, "Schedule type", c.ScheduleType)
	kv(w, "Materialization policy", c.MaterializationPolicy)
	kv(w, "Concurrency policy", c.ConcurrencyPolicy)
	kv(w, "Active", fmt.Sprintf("%v", c.Active))
	kv(w, "Run once done", fmt.Sprintf("%v", c.RunOnceDone))
	if c.NextRunAt != "" {
		kv(w, "Next run at", c.NextRunAt)
	}
	kv(w, "Created", c.CreatedAt)
	kv(w, "Updated", c.UpdatedAt)
	fmt.Fprintln(w)
}

// PrintCampaignRunList writes runs table.
func PrintCampaignRunList(w io.Writer, r *client.CampaignRunListResponse) {
	if r == nil || len(r.Items) == 0 {
		fmt.Fprintln(w, "  No runs.")
		return
	}
	section(w, "Runs")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  ID\tSTATUS\tDISPATCH_REF\tMAT_ID\tCREATED_AT")
	fmt.Fprintln(tw, "  ──\t──────\t────────────\t──────\t──────────")
	for _, run := range r.Items {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", run.ID, run.Status, run.DispatchRef, run.TargetMaterializationID, run.CreatedAt)
	}
	tw.Flush()
	if r.HasMore {
		fmt.Fprintf(w, "\n  … more (total %d)\n", r.Total)
	}
	fmt.Fprintln(w)
}

// PrintCampaignRunGet writes a single run (or launch result).
func PrintCampaignRunGet(w io.Writer, r *client.CampaignRunResponse) {
	if r == nil {
		return
	}
	section(w, "Run · "+r.ID)
	kv(w, "ID", r.ID)
	kv(w, "Campaign ID", r.CampaignID)
	kv(w, "Status", r.Status)
	kv(w, "Target ID", r.TargetID)
	kv(w, "Target materialization ID", r.TargetMaterializationID)
	kv(w, "Scan profile slug", r.ScanProfileSlug)
	kv(w, "Dispatch ref", r.DispatchRef)
	if r.CreatedAt != "" {
		kv(w, "Created", r.CreatedAt)
	}
	if r.DispatchedAt != "" {
		kv(w, "Dispatched at", r.DispatchedAt)
	}
	if r.ErrorMessage != "" {
		kv(w, "Error", r.ErrorMessage)
	}
	if r.Stats != nil {
		kv(w, "Stats", fmt.Sprintf("%v", r.Stats))
	}
	fmt.Fprintln(w)
}

// --- execution / worker (execution-service) ---

// PrintExecutionList writes executions table.
func PrintExecutionList(w io.Writer, r *client.ExecutionListResponse) {
	if r == nil || len(r.Items) == 0 {
		fmt.Fprintln(w, "  No executions.")
		return
	}
	section(w, "Executions")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  ID\tRUN_ID\tCAMPAIGN_ID\tSTATUS\tJOBS\tCOMPLETED\tFAILED\tCREATED_AT")
	fmt.Fprintln(tw, "  ──\t──────\t───────────\t──────\t────\t─────────\t─────\t──────────")
	for _, e := range r.Items {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%d\t%d\t%d\t%s\n",
			e.ID,
			e.RunID,
			e.CampaignID,
			e.Status,
			e.TotalJobs,
			e.CompletedJobs,
			e.FailedJobs,
			e.CreatedAt,
		)
	}
	tw.Flush()
	fmt.Fprintf(w, "\n  Total: %d (limit %d, offset %d)\n\n", r.Total, r.Limit, r.Offset)
}

// PrintExecution writes one execution (get).
func PrintExecution(w io.Writer, e *client.ExecutionItem) {
	if e == nil {
		return
	}
	section(w, "Execution · "+e.ID)
	kv(w, "ID", e.ID)
	kv(w, "Run ID", e.RunID)
	kv(w, "Campaign ID", e.CampaignID)
	kv(w, "Target ID", e.TargetID)
	kv(w, "Target materialization ID", e.TargetMaterializationID)
	kv(w, "Scan profile", e.ScanProfileSlug)
	kv(w, "Status", e.Status)
	kv(w, "Total jobs", fmt.Sprintf("%d", e.TotalJobs))
	kv(w, "Completed", fmt.Sprintf("%d", e.CompletedJobs))
	kv(w, "Failed", fmt.Sprintf("%d", e.FailedJobs))
	kv(w, "Created", e.CreatedAt)
	kv(w, "Updated", e.UpdatedAt)
	if e.CompletedAt != nil {
		kv(w, "Completed at", *e.CompletedAt)
	}
	if e.ErrorSummary != "" {
		kv(w, "Error summary", e.ErrorSummary)
	}
	fmt.Fprintln(w)
}

// PrintExecutionJobs writes jobs table (execution jobs or worker jobs).
func PrintExecutionJobs(w io.Writer, items []client.ExecutionJobItem, total, limit, offset int, title string) {
	if len(items) == 0 {
		fmt.Fprintln(w, "  No jobs.")
		return
	}
	if title != "" {
		section(w, title)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  ID\tEXECUTION_ID\tSTATUS\tATTEMPT\tMAX\tWORKER\tLEASE_EXPIRES\tERROR")
	fmt.Fprintln(tw, "  ──\t────────────\t──────\t───────\t───\t──────\t────────────\t─────")
	for _, j := range items {
		worker := "—"
		if j.AssignedWorkerID != nil {
			worker = *j.AssignedWorkerID
		}
		expires := "—"
		if j.LeaseExpiresAt != nil {
			expires = *j.LeaseExpiresAt
		}
		errStr := trunc(j.ErrorMessage, 20)
		if errStr == "" {
			errStr = "—"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%d\t%d\t%s\t%s\t%s\n",
			j.ID,
			j.ExecutionID,
			j.Status,
			j.Attempt,
			j.MaxAttempts,
			worker,
			expires,
			errStr,
		)
	}
	tw.Flush()
	if total >= 0 && (limit > 0 || offset > 0) {
		fmt.Fprintf(w, "\n  Total: %d\n", total)
	}
	fmt.Fprintln(w)
}

// PrintWorkerList writes workers table (full ID so you can copy for worker jobs <id>).
func PrintWorkerList(w io.Writer, r *client.WorkerListResponse) {
	if r == nil || len(r.Items) == 0 {
		fmt.Fprintln(w, "  No workers.")
		return
	}
	section(w, "Workers")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  ID\tNAME\tREGION\tVERSION\tSTATUS\tLOAD\tMAX_CONC\tLAST_HEARTBEAT")
	fmt.Fprintln(tw, "  ──\t────\t──────\t───────\t──────\t────\t────────\t──────────────")
	for _, w := range r.Items {
		hb := "—"
		if w.LastHeartbeatAt != nil {
			hb = *w.LastHeartbeatAt
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\n",
			w.ID,
			w.Name,
			w.Region,
			w.Version,
			w.Status,
			w.CurrentLoad,
			w.MaxConcurrency,
			hb,
		)
	}
	tw.Flush()
	fmt.Fprintf(w, "\n  Total: %d (limit %d, offset %d)\n\n", r.Total, r.Limit, r.Offset)
}

// PrintWorker writes one worker (get).
func PrintWorker(w io.Writer, wkr *client.WorkerItem) {
	if wkr == nil {
		return
	}
	section(w, "Worker · "+wkr.Name)
	kv(w, "ID", wkr.ID)
	kv(w, "Name", wkr.Name)
	kv(w, "Region", wkr.Region)
	kv(w, "Version", wkr.Version)
	kv(w, "Status", wkr.Status)
	kv(w, "Current load", fmt.Sprintf("%d", wkr.CurrentLoad))
	kv(w, "Max concurrency", fmt.Sprintf("%d", wkr.MaxConcurrency))
	kv(w, "Capabilities", strings.Join(wkr.Capabilities, ", "))
	if wkr.LastHeartbeatAt != nil {
		kv(w, "Last heartbeat", *wkr.LastHeartbeatAt)
	}
	kv(w, "Created", wkr.CreatedAt)
	kv(w, "Updated", wkr.UpdatedAt)
	fmt.Fprintln(w)
}
