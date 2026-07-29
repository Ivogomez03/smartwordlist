// Package recon provides target intelligence gathering for the SmartWordlist
// pipeline. It coordinates HTML scraping, DNS enumeration, and robots/sitemap
// discovery through a fan-in concurrency model where each sub-collector runs
// in its own goroutine and partial failures never block the overall result.
package recon

import (
	"context"
	"fmt"
	"sync"

	"github.com/gentleman-programming/smartwordlist/pkg/types"
)

// ReconCollector orchestrates all reconnaissance sub-collectors and
// implements the ReconCollector interface from the pipeline design.
// A zero value is not usable; construct via NewReconCollector.
type ReconCollector struct{}

// NewReconCollector returns a ready-to-use ReconCollector.
func NewReconCollector() *ReconCollector {
	return &ReconCollector{}
}

// Collect runs every sub-collector (scrape, DNS, robots) concurrently and
// merges their results into a single ReconResult. Each sub-collector runs
// in its own goroutine; errors from one do not block the others (partial
// failure tolerance). The returned result may contain partial data when
// some collectors fail.
//
// The context controls cancellation, though sub-collectors honour it on a
// best-effort basis (colly and net.LookupHost do not accept contexts
// directly — the goroutine is the isolation boundary).
func (rc *ReconCollector) Collect(ctx context.Context, domain string) (*types.ReconResult, error) {
	result := &types.ReconResult{}

	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := make([]error, 0, 3)

	// ---- scrape goroutine ----
	wg.Add(1)
	go func() {
		defer wg.Done()
		scrapeRes, err := scrapeHTML(ctx, domain)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("scrape: %w", err))
			return
		}
		result.Title = scrapeRes.Title
		result.Company = scrapeRes.Company
		result.Keywords = scrapeRes.Keywords
		result.Technologies = scrapeRes.Technologies
		result.Emails = scrapeRes.Emails
	}()

	// ---- DNS goroutine ----
	wg.Add(1)
	go func() {
		defer wg.Done()
		subdomains, err := enumerateDNS(ctx, domain)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("dns: %w", err))
			return
		}
		result.Subdomains = subdomains
	}()

	// ---- robots goroutine ----
	wg.Add(1)
	go func() {
		defer wg.Done()
		paths, err := fetchRobotsAndSitemap(ctx, domain)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("robots: %w", err))
			return
		}
		result.Paths = paths
	}()

	wg.Wait()

	// If every sub-collector failed, surface the combined errors.
	// Otherwise, return the partial result (this is by design).
	if len(errs) == 3 {
		return nil, fmt.Errorf("all collectors failed: %v", errs)
	}

	return result, nil
}
