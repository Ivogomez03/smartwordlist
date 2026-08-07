package recon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// commonSubdomains are prefixed to the target domain during DNS enumeration.
var commonSubdomains = []string{
	"www", "mail", "admin", "dev", "api", "staging",
	"test", "blog", "shop", "app", "cdn", "remote",
	"vpn", "portal", "dashboard",
}

// dnsTimeout is the maximum time a single LookupHost call may block.
const dnsTimeout = 3 * time.Second

// crtShURL is the Certificate Transparency API endpoint for subdomain discovery.
const crtShURL = "https://crt.sh/?q=%%.%s&output=json"

// crtShEntry is a single row from the crt.sh JSON output.
type crtShEntry struct {
	NameValue string `json:"name_value"`
}

// enumerateDNS resolves common subdomain prefixes and supplements results
// with certificate transparency data from crt.sh. Errors from individual
// lookups are swallowed — only successfully resolved hosts are returned.
func enumerateDNS(ctx context.Context, domain string) ([]string, error) {
	var (
		found []string
		mu    sync.Mutex
		wg    sync.WaitGroup
	)

	// ---- DNS resolution of common prefixes ----
	for _, prefix := range commonSubdomains {
		// Check context before spawning goroutines.
		select {
		case <-ctx.Done():
			return deduplicateStrings(found), ctx.Err()
		default:
		}

		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			host := p + "." + domain
			if resolves(ctx, host) {
				mu.Lock()
				found = append(found, host)
				mu.Unlock()
			}
		}(prefix)
	}

	wg.Wait()

	// ---- crt.sh fallback ----
	if subdomains, err := crtShLookup(ctx, domain); err == nil {
		mu.Lock()
		found = append(found, subdomains...)
		mu.Unlock()
	}
	// Errors from crt.sh are intentionally swallowed — the DNS results
	// are still valid partial data.

	return deduplicateStrings(found), nil
}

// resolves returns true when a hostname resolves to at least one IP address.
// It uses net.DefaultResolver.LookupHost, which honours the context deadline
// natively — unlike the bare net.LookupHost, this actually aborts the
// in-flight lookup on timeout/cancellation instead of leaking a goroutine
// that lingers until the OS resolver eventually gives up on its own.
func resolves(ctx context.Context, host string) bool {
	lookupCtx, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupHost(lookupCtx, host)
	return err == nil && len(addrs) > 0
}

// crtShLookup queries crt.sh for subdomains listed in certificate
// transparency logs and returns the unique set of subdomains found.
func crtShLookup(ctx context.Context, domain string) ([]string, error) {
	url := fmt.Sprintf(crtShURL, domain)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("crt.sh: build request: %w", err)
	}
	req.Header.Set("User-Agent", "SmartWordlist/0.1")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crt.sh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("crt.sh: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var entries []crtShEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		// crt.sh sometimes returns HTML on errors — treat as empty result.
		return nil, fmt.Errorf("crt.sh: decode: %w", err)
	}

	subdomains := make([]string, 0, len(entries))
	for _, entry := range entries {
		for _, name := range strings.Split(entry.NameValue, "\n") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			// Filter out wildcard entries and keep only names that
			// belong to the target domain.
			lower := strings.ToLower(name)
			if strings.Contains(lower, "*") {
				continue
			}
			if !strings.HasSuffix(lower, "."+strings.ToLower(domain)) && lower != strings.ToLower(domain) {
				continue
			}
			subdomains = append(subdomains, name)
		}
	}

	return deduplicateStrings(subdomains), nil
}
