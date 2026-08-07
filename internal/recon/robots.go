package recon

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// robotsTimeout is the HTTP client timeout for fetching robots and sitemap.
const robotsTimeout = 10 * time.Second

// sitemapIndex is a minimal XML structure for parsing sitemap index files
// that list child sitemaps.
type sitemapIndex struct {
	XMLName  xml.Name     `xml:"sitemapindex"`
	Sitemaps []sitemapLoc `xml:"sitemap"`
}

// urlSet is a minimal XML structure for a standard urlset sitemap.
type urlSet struct {
	XMLName xml.Name     `xml:"urlset"`
	URLs    []sitemapLoc `xml:"url"`
}

// sitemapLoc holds a <loc> element from either a sitemap index or urlset.
type sitemapLoc struct {
	Loc string `xml:"loc"`
}

// fetchRobotsAndSitemap fetches robots.txt and sitemap.xml for the given
// domain. A 404 or connection error for either resource is not treated as a
// failure — the function returns whatever paths were discovered. Only when
// both resources fail unrecoverably is an error returned.
func fetchRobotsAndSitemap(ctx context.Context, domain string) ([]string, error) {
	var paths []string

	// robots.txt — failure is non-fatal per spec.
	if robotPaths, err := parseRobotsTxt(ctx, domain); err == nil {
		paths = append(paths, robotPaths...)
	}
	// If robots.txt errors, there may still be sitemap data.

	// sitemap.xml — failure is non-fatal per spec.
	if sitemapPaths, err := parseSitemap(ctx, domain); err == nil {
		paths = append(paths, sitemapPaths...)
	}

	return deduplicateStrings(paths), nil
}

// parseRobotsTxt fetches https://{domain}/robots.txt and extracts the path
// portion of every Disallow and Allow directive.
func parseRobotsTxt(ctx context.Context, domain string) ([]string, error) {
	url := "https://" + domain + "/robots.txt"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("robots.txt: build request: %w", err)
	}
	req.Header.Set("User-Agent", "SmartWordlist/0.1")

	client := &http.Client{Timeout: robotsTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("robots.txt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // graceful: no robots.txt is not an error
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("robots.txt: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var paths []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Normalise: lowercase, strip inline comments.
		lower := strings.ToLower(line)
		if commentIdx := strings.IndexByte(lower, '#'); commentIdx >= 0 {
			lower = lower[:commentIdx]
		}

		fields := strings.Fields(lower)
		if len(fields) < 2 {
			continue
		}

		directive := fields[0]
		value := fields[1]

		if directive == "disallow" || directive == "allow" {
			if value != "" && value != "/" {
				paths = append(paths, value)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return paths, fmt.Errorf("robots.txt: scan: %w", err)
	}

	return paths, nil
}

// parseSitemap fetches https://{domain}/sitemap.xml and extracts all <loc>
// URLs. It handles both standard urlset sitemaps and sitemap index files
// that point to child sitemaps (one level deep only — MVP scope).
func parseSitemap(ctx context.Context, domain string) ([]string, error) {
	url := "https://" + domain + "/sitemap.xml"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("sitemap: build request: %w", err)
	}
	req.Header.Set("User-Agent", "SmartWordlist/0.1")

	client := &http.Client{Timeout: robotsTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sitemap: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // graceful: no sitemap is not an error
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("sitemap: HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sitemap: read body: %w", err)
	}

	var paths []string

	// Try sitemap index first (lists child sitemaps).
	var index sitemapIndex
	if err := xml.Unmarshal(body, &index); err == nil && len(index.Sitemaps) > 0 {
		// Fetch first-level child sitemaps (MVP: one level deep only).
		for _, sm := range index.Sitemaps {
			if sm.Loc == "" {
				continue
			}
			childPaths, childErr := fetchChildSitemap(ctx, sm.Loc)
			if childErr == nil {
				paths = append(paths, childPaths...)
			}
			// Ignore child sitemap errors — partial data is OK.
		}
	}

	// Try standard urlset.
	var urlset urlSet
	if err := xml.Unmarshal(body, &urlset); err == nil && len(urlset.URLs) > 0 {
		for _, u := range urlset.URLs {
			if u.Loc != "" {
				paths = append(paths, u.Loc)
			}
		}
	}

	return paths, nil
}

// fetchChildSitemap fetches a single child sitemap URL and extracts <loc> entries.
func fetchChildSitemap(ctx context.Context, url string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("child sitemap: build request: %w", err)
	}
	req.Header.Set("User-Agent", "SmartWordlist/0.1")

	client := &http.Client{Timeout: robotsTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("child sitemap: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("child sitemap: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("child sitemap: read body: %w", err)
	}

	var urlset urlSet
	if err := xml.Unmarshal(body, &urlset); err != nil {
		return nil, fmt.Errorf("child sitemap: decode: %w", err)
	}

	paths := make([]string, 0, len(urlset.URLs))
	for _, u := range urlset.URLs {
		if u.Loc != "" {
			paths = append(paths, u.Loc)
		}
	}

	return paths, nil
}
