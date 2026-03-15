package cmd

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/client"
	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/output"
	"github.com/spf13/cobra"
)

const (
	installPollInterval = 15 * time.Second
	installPollTimeout  = 10 * time.Minute
	installClientTimeout = 15 * time.Minute
)

// dataset state that we consider "ready" for sync (dataset-service uses "validated", not "ready"/"imported")
const installReadyState = "validated"

var (
	installSkipWait bool
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Download all datasets (RIR + CAIDA) and sync scope + routing",
	Long: `Downloads RIR (all registries) and CAIDA (pfx2as IPv4/IPv6, as-org), waits for at least one dataset to be validated, then runs scope sync and routing sync in parallel.

Steps:
  1) POST fetch RIR (all)
  2) POST fetch CAIDA sources (parallel)
  3) Poll dataset list until at least one is validated (or --skip-wait)
  4) Scope sync + Routing sync (parallel)

To re-run only sync (e.g. after fixing network): use --skip-wait to skip the wait step.
To start completely fresh: reset dataset-service data (DB + storage) then run install again.`,
	RunE:  runInstall,
}

func init() {
	installCmd.Flags().BoolVar(&installSkipWait, "skip-wait", false, "Skip waiting for datasets; run scope+routing sync with whatever is already validated")
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	cfg := getConfig()
	cl := client.New(cfg).WithTimeout(installClientTimeout)
	ctx := context.Background()

	if isJSON() {
		return runInstallJSON(ctx, cl)
	}
	return runInstallTable(ctx, cl)
}

func runInstallTable(ctx context.Context, cl *client.Client) error {
	const totalSteps = 5
	step := func(n int, msg string) {
		fmt.Fprintf(os.Stdout, "\n  [Step %d/%d] %s\n", n, totalSteps, msg)
	}

	// --- Step 1: Fetch RIR (all) ---
	step(1, "Requesting RIR fetch (all registries)...")
	rirResult, err := cl.DatasetFetch(ctx, "all", "")
	if err != nil {
		return fmt.Errorf("dataset fetch RIR: %w", err)
	}
	output.PrintFetchResult(os.Stdout, rirResult)

	// --- Step 2: Fetch CAIDA ---
	step(2, "Requesting CAIDA fetch (pfx2as IPv4/IPv6, as-org)...")
	caidaSources := []string{"caida_pfx2as_ipv4", "caida_pfx2as_ipv6", "caida_as_org"}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var caidaErrs []error
	for _, src := range caidaSources {
		src := src
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cl.DatasetFetch(ctx, "", src)
			if err != nil {
				mu.Lock()
				caidaErrs = append(caidaErrs, fmt.Errorf("%s: %w", src, err))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(caidaErrs) > 0 {
		return fmt.Errorf("dataset fetch CAIDA: %v", caidaErrs)
	}
	fmt.Fprintln(os.Stdout, "  CAIDA fetch requests sent.")

	// --- Step 3: Wait for at least one validated (or skip) ---
	if installSkipWait {
		step(3, "Skipping wait (--skip-wait). Using currently validated datasets.")
	} else {
		step(3, "Waiting for datasets to become validated (polling every 15s, max 10m)...")
		deadline := time.Now().Add(installPollTimeout)
		startWait := time.Now()
		attempt := 0
		for time.Now().Before(deadline) {
			attempt++
			elapsed := time.Since(startWait).Round(time.Second)
			list, err := cl.DatasetList(ctx, "", "")
			if err != nil {
				return fmt.Errorf("dataset list: %w", err)
			}
			var validated, fetching, fetched, failed, rejected int
			for _, d := range list.Datasets {
				switch d.State {
				case installReadyState:
					validated++
				case "fetching":
					fetching++
				case "fetched":
					fetched++
				case "failed":
					failed++
				case "rejected":
					rejected++
				default:
					// created, etc.
					fetching++
				}
			}
			fmt.Fprintf(os.Stdout, "    attempt %d (elapsed %s) — validated: %d, fetching/fetched: %d, failed: %d, rejected: %d\n",
				attempt, elapsed, validated, fetching+fetched, failed, rejected)
			if validated > 0 {
				fmt.Fprintf(os.Stdout, "  At least one dataset validated. Proceeding.\n")
				break
			}
			time.Sleep(installPollInterval)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %v: no dataset reached state %q. Use --skip-wait to run sync with current data, or check dataset-service and network", installPollTimeout, installReadyState)
		}
	}

	// --- Step 4 & 5: Scope sync + Routing sync (parallel) ---
	fmt.Fprintf(os.Stdout, "\n  [Step 4/%d] Triggering scope sync...\n", totalSteps)
	fmt.Fprintf(os.Stdout, "  [Step 5/%d] Triggering routing sync...\n", totalSteps)
	fmt.Fprintln(os.Stdout, "  (running in parallel)")
	var scopeResp *client.ScopeSyncResponse
	var routingResp *client.RoutingSyncResponse
	var scopeErr, routingErr error
	wg = sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		scopeResp, scopeErr = cl.ScopeSync(ctx)
	}()
	go func() {
		defer wg.Done()
		routingResp, routingErr = cl.RoutingSync(ctx)
	}()
	wg.Wait()

	if scopeErr != nil {
		return fmt.Errorf("scope sync: %w", scopeErr)
	}
	if routingErr != nil {
		return fmt.Errorf("routing sync: %w", routingErr)
	}

	output.PrintScopeSync(os.Stdout, scopeResp)
	output.PrintRoutingSync(os.Stdout, routingResp)
	fmt.Fprintln(os.Stdout, "\n  Install complete.")
	return nil
}

func runInstallJSON(ctx context.Context, cl *client.Client) error {
	rirResult, err := cl.DatasetFetch(ctx, "all", "")
	if err != nil {
		return fmt.Errorf("dataset fetch RIR: %w", err)
	}
	_ = output.PrintJSON(os.Stdout, rirResult)

	caidaSources := []string{"caida_pfx2as_ipv4", "caida_pfx2as_ipv6", "caida_as_org"}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var caidaErrs []error
	for _, src := range caidaSources {
		src := src
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cl.DatasetFetch(ctx, "", src)
			if err != nil {
				mu.Lock()
				caidaErrs = append(caidaErrs, fmt.Errorf("%s: %w", src, err))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(caidaErrs) > 0 {
		return fmt.Errorf("dataset fetch CAIDA: %v", caidaErrs)
	}

	if !installSkipWait {
		deadline := time.Now().Add(installPollTimeout)
		for time.Now().Before(deadline) {
			list, err := cl.DatasetList(ctx, "", "")
			if err != nil {
				return fmt.Errorf("dataset list: %w", err)
			}
			validated := 0
			for _, d := range list.Datasets {
				if d.State == installReadyState {
					validated++
				}
			}
			if validated > 0 {
				break
			}
			time.Sleep(installPollInterval)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for datasets (no validated after %v)", installPollTimeout)
		}
	}

	var scopeResp *client.ScopeSyncResponse
	var routingResp *client.RoutingSyncResponse
	var scopeErr, routingErr error
	wg = sync.WaitGroup{}
	wg.Add(2)
	go func() { defer wg.Done(); scopeResp, scopeErr = cl.ScopeSync(ctx) }()
	go func() { defer wg.Done(); routingResp, routingErr = cl.RoutingSync(ctx) }()
	wg.Wait()
	if scopeErr != nil {
		return fmt.Errorf("scope sync: %w", scopeErr)
	}
	if routingErr != nil {
		return fmt.Errorf("routing sync: %w", routingErr)
	}
	type installResult struct {
		Scope   *client.ScopeSyncResponse   `json:"scope"`
		Routing *client.RoutingSyncResponse `json:"routing"`
	}
	return output.PrintJSON(os.Stdout, &installResult{Scope: scopeResp, Routing: routingResp})
}
