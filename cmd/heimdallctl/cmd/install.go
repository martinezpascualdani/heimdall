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

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Download all datasets (RIR + CAIDA) and sync scope + routing",
	Long:  "Fetches RIR (all registries) and CAIDA (pfx2as IPv4/IPv6, as-org), waits for datasets to be ready, then runs scope sync and routing sync in parallel.",
	RunE:  runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	cfg := getConfig()
	cl := client.New(cfg).WithTimeout(installClientTimeout)
	ctx := context.Background()

	// --- 1. Fetch RIR (all) ---
	fmt.Fprintln(os.Stdout, "  → Fetching RIR (all registries)...")
	rirResult, err := cl.DatasetFetch(ctx, "all", "")
	if err != nil {
		return fmt.Errorf("dataset fetch RIR: %w", err)
	}
	if isJSON() {
		_ = output.PrintJSON(os.Stdout, rirResult)
	} else {
		output.PrintFetchResult(os.Stdout, rirResult)
	}

	// --- 2. Fetch CAIDA sources in parallel ---
	caidaSources := []string{"caida_pfx2as_ipv4", "caida_pfx2as_ipv6", "caida_as_org"}
	fmt.Fprintf(os.Stdout, "  → Fetching CAIDA (%v)...\n", caidaSources)
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
	if !isJSON() {
		fmt.Fprintln(os.Stdout, "  CAIDA fetch requests sent.")
	}

	// --- 3. Wait for datasets to be ready ---
	fmt.Fprintln(os.Stdout, "  → Waiting for datasets to be ready (polling)...")
	deadline := time.Now().Add(installPollTimeout)
	for time.Now().Before(deadline) {
		list, err := cl.DatasetList(ctx, "", "")
		if err != nil {
			return fmt.Errorf("dataset list: %w", err)
		}
		ready := 0
		for _, d := range list.Datasets {
			if d.State == "ready" || d.State == "imported" {
				ready++
			}
		}
		if ready > 0 {
			if !isJSON() {
				fmt.Fprintf(os.Stdout, "  Datasets ready: %d (continuing).\n", ready)
			}
			break
		}
		time.Sleep(installPollInterval)
	}
	if time.Now().After(deadline) {
		return fmt.Errorf("timeout waiting for datasets (no ready state after %v)", installPollTimeout)
	}

	// --- 4. Scope sync + Routing sync in parallel ---
	fmt.Fprintln(os.Stdout, "  → Syncing scope and routing (parallel)...")
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

	if isJSON() {
		type installResult struct {
			Scope   *client.ScopeSyncResponse   `json:"scope"`
			Routing *client.RoutingSyncResponse `json:"routing"`
		}
		return output.PrintJSON(os.Stdout, &installResult{Scope: scopeResp, Routing: routingResp})
	}
	output.PrintScopeSync(os.Stdout, scopeResp)
	output.PrintRoutingSync(os.Stdout, routingResp)
	fmt.Fprintln(os.Stdout, "  Install complete.")
	return nil
}
