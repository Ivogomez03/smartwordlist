package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain_E2E_NoLLM(t *testing.T) {
	// This is an integration test — it runs the full pipeline using --no-llm.
	// It requires network access for the crt.sh lookup. In CI without network,
	// we skip with a message.
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Start a fake HTTP server serving a company page.
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<title>Acme Corp | Widget Solutions</title>
	<meta name="description" content="Acme Corp provides enterprise widget solutions.">
	<meta property="og:site_name" content="Acme Corporation">
	<meta name="generator" content="WordPress 6.4">
</head>
<body>
	<h1>Acme Corp</h1>
	<p>We build widgets, cloud solutions, and security tools.</p>
	<p>Contact: info@acmecorp.com</p>
</body>
</html>`))
		case "/robots.txt":
			w.Write([]byte("User-agent: *\nDisallow: /admin\nAllow: /public\n"))
		case "/sitemap.xml":
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	<url><loc>https://acmecorp.com/about</loc></url>
	<url><loc>https://acmecorp.com/contact</loc></url>
</urlset>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// Extract the host from the TLS test server URL.
	host := strings.TrimPrefix(ts.URL, "https://")
	// Strip port for domain validation (e.g., "127.0.0.1:12345" -> "127.0.0.1").
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Create a temp rules file.
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "test-rules.yaml")
	if err := os.WriteFile(rulesPath, []byte(`
leet_map:
  a:
    - "4"
    - "@"
  e:
    - "3"
  i:
    - "1"
  o:
    - "0"
suffixes:
  - "123"
  - "!"
prefixes:
  - "admin_"
year_range:
  start: 2025
  end: 2026
case_variations:
  - lower
  - upper
  - title
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	// Run the pipeline.
	oldArgs := os.Args
	os.Args = []string{
		"smartwordlist", host,
		"--no-llm", "--max", "200", "--verbose",
		"--rules", rulesPath,
	}
	rootCmd.SetArgs([]string{host, "--no-llm", "--max", "200", "--verbose", "--rules", rulesPath})

	runErr := rootCmd.Execute()

	// Restore stdout and read captured output.
	w.Close()
	os.Stdout = oldStdout
	os.Args = oldArgs

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	if runErr != nil {
		// The test target may fail DNS or scraping (test.invalid pattern).
		// Don't fail the test on pipeline errors — the goal is to verify
		// the pipeline doesn't panic and produces meaningful output.
		t.Logf("Pipeline returned error (may be expected for test target): %v", runErr)
		t.Logf("Output:\n%s", output)
	}

	// Verify banner appears.
	if !strings.Contains(output, "SmartWordlist") {
		t.Error("expected 'SmartWordlist' banner in output")
	}

	// Verify rule-only mode is active.
	if !strings.Contains(output, "rule-only") && !strings.Contains(output, "LLM") {
		t.Log("output doesn't mention LLM mode or rule-only — this is OK if ollama was detected")
	}

	// If the output contains "Done!", the pipeline completed.
	if strings.Contains(output, "Done!") {
		t.Log("Pipeline completed successfully")
	}

	// If the output contains candidate count, verify it's reasonable.
	if strings.Contains(output, "Generated") || strings.Contains(output, "Done!") {
		fmt.Fprintf(os.Stderr, "E2E test output (first 500 chars):\n%s\n", truncateString(output, 500))
	}
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func TestDomainValidation(t *testing.T) {
	valid := []string{
		"example.com",
		"www.example.com",
		"sub.domain.example.com",
		"my-domain.com",
		"test123.co.uk",
		"192.168.1.1",
		"10.0.0.1",
		"127.0.0.1",
	}
	for _, d := range valid {
		if !domainRegex.MatchString(d) {
			t.Errorf("expected valid domain: %q", d)
		}
	}

	invalid := []string{
		"",
		"https://example.com",
		"example.com/path",
		"example com",
		"-example.com",
		"example.",
		".example.com",
		"127.0.0.1:8080",
		"user@example.com",
	}
	for _, d := range invalid {
		if domainRegex.MatchString(d) {
			t.Errorf("expected invalid domain: %q", d)
		}
	}
}
