package recon

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testTransport returns an http.RoundTripper that rewrites all requests to
// the given httptest server and skips TLS verification (needed because
// httptest TLS certs are self-signed).
func testTransport(ts *httptest.Server) http.RoundTripper {
	return &rewritingTransport{
		base: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		rewrite: ts.URL,
	}
}

type rewritingTransport struct {
	base    http.RoundTripper
	rewrite string
}

func (rt *rewritingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Build URL pointing at the test server, keeping the original path.
	u := rt.rewrite + req.URL.Path
	if req.URL.RawQuery != "" {
		u += "?" + req.URL.RawQuery
	}
	clone, err := http.NewRequestWithContext(req.Context(), req.Method, u, req.Body)
	if err != nil {
		return nil, err
	}
	clone.Header = req.Header
	return rt.base.RoundTrip(clone)
}

func TestScrapeHTML_ExtractsSpanishSite(t *testing.T) {
	// Simulates a real Spanish-language site like cerrobayo.com.ar.
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Server", "nginx/1.24")
		w.Header().Set("Set-Cookie", "PHPSESSID=abc123; Path=/")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html>
<html dir="ltr" lang="es">
<head>
	<meta http-equiv="content-type" content="text/html; charset=utf-8" />
	<meta name="author" content="Cerro Bayo" />
	<meta name="description" content="Encontrá tu centro! Viví la nieve, gastronomía de altura y eventos exclusivos, con la mejor vista de la Patagonia." />
	<meta property="og:title" content="Cerro Bayo Ski Boutique" />
	<meta property="og:description" content="Encontrá tu centro! Viví la nieve, gastronomía de altura y eventos exclusivos." />
	<meta property="og:url" content="https://www.cerrobayo.com.ar/" />
	<meta property="fb:app_id" content="1388756364765747" />
	<meta name="generator" content="Joomla! 4" />
	<title></title>
</head>
<body>
	<h1>Cerro Bayo — Ski Boutique en la Patagonia</h1>
	<p>Disfrutá de la mejor nieve, gastronomía de altura y eventos exclusivos.</p>
	<p>Contacto: info@cerrobayo.com.ar</p>
	<script src="/media/vendor/jquery/jquery.min.js"></script>
	<script src="/templates/cerrobayo/js/bootstrap.bundle.min.js"></script>
	<script src="/media/system/js/vue.js"></script>
</body>
</html>`))
	}))
	defer ts.Close()

	// Inject transport so colly hits our test server regardless of domain.
	scrapeTransport = testTransport(ts)
	defer func() { scrapeTransport = nil }()

	ctx := context.Background()
	result, err := scrapeHTML(ctx, "cerrobayo.com.ar")
	if err != nil {
		t.Fatalf("scrapeHTML failed: %v", err)
	}

	if result.Title != "Cerro Bayo Ski Boutique" {
		t.Errorf("Title = %q, want %q", result.Title, "Cerro Bayo Ski Boutique")
	}
	if result.Company != "Cerro Bayo" {
		t.Errorf("Company = %q, want %q", result.Company, "Cerro Bayo")
	}
	if len(result.Keywords) == 0 {
		t.Error("Keywords should not be empty")
	}
	foundNieve := false
	foundPatagonia := false
	for _, kw := range result.Keywords {
		if strings.EqualFold(kw, "nieve") {
			foundNieve = true
		}
		if strings.EqualFold(kw, "patagonia") {
			foundPatagonia = true
		}
	}
	if !foundNieve {
		t.Errorf("Keywords should contain 'nieve', got: %v", result.Keywords)
	}
	if !foundPatagonia {
		t.Errorf("Keywords should contain 'patagonia', got: %v", result.Keywords)
	}

	techStr := strings.Join(result.Technologies, " ")
	for _, want := range []string{"Joomla! 4", "jQuery", "Bootstrap", "Vue.js", "nginx"} {
		if !strings.Contains(strings.ToLower(techStr), strings.ToLower(want)) {
			t.Errorf("Technologies should contain %q, got: %s", want, techStr)
		}
	}

	if len(result.Emails) == 0 {
		t.Error("Should have found at least one email")
	}
	foundEmail := false
	for _, e := range result.Emails {
		if e == "info@cerrobayo.com.ar" {
			foundEmail = true
			break
		}
	}
	if !foundEmail {
		t.Errorf("Should have found info@cerrobayo.com.ar, got: %v", result.Emails)
	}
}

func TestScrapeHTML_HTTPError(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	scrapeTransport = testTransport(ts)
	defer func() { scrapeTransport = nil }()

	ctx := context.Background()
	_, err := scrapeHTML(ctx, "failing.example.com")
	if err == nil {
		t.Fatal("Expected error for HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Error should mention status 500, got: %v", err)
	}
}

func TestScrapeHTML_ConnectionError(t *testing.T) {
	ctx := context.Background()
	_, err := scrapeHTML(ctx, "127.0.0.1:1")
	if err == nil {
		t.Fatal("Expected error for connection refused, got nil")
	}
	t.Logf("Connection error (expected): %v", err)
}

func TestScrapeHTML_ExtractsCompanyFromTitleFallback(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<title>EmpresaX S.A. | Soluciones Tecnológicas</title>
</head>
<body><p>Contenido.</p></body>
</html>`))
	}))
	defer ts.Close()

	scrapeTransport = testTransport(ts)
	defer func() { scrapeTransport = nil }()

	ctx := context.Background()
	result, err := scrapeHTML(ctx, "empresax.com.ar")
	if err != nil {
		t.Fatalf("scrapeHTML failed: %v", err)
	}

	if result.Company != "EmpresaX S.A." {
		t.Errorf("Company = %q, want %q", result.Company, "EmpresaX S.A.")
	}
}

func TestScrapeHTML_NoContent(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	scrapeTransport = testTransport(ts)
	defer func() { scrapeTransport = nil }()

	ctx := context.Background()
	result, err := scrapeHTML(ctx, "empty.example.com")
	if err != nil {
		t.Fatalf("scrapeHTML should not error on empty 200: %v", err)
	}
	if result.Title != "" {
		t.Errorf("Title should be empty, got %q", result.Title)
	}
	if result.Company != "" {
		t.Errorf("Company should be empty, got %q", result.Company)
	}
}

// TestScrapeHTML_HonoursContextCancellation is the regression guard for a
// bug where scrapeHTML accepted a context.Context but never actually used it
// to bound colly's Visit call, so a slow/unresponsive target would hang past
// --pipeline-timeout regardless of the configured deadline.
func TestScrapeHTML_HonoursContextCancellation(t *testing.T) {
	unblock := make(chan struct{})
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-unblock // hang until the test explicitly releases it
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	defer close(unblock)

	scrapeTransport = testTransport(ts)
	defer func() { scrapeTransport = nil }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := scrapeHTML(ctx, "slow.example.com")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error when the context deadline is exceeded")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected error to wrap context.DeadlineExceeded, got: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("scrapeHTML should return promptly on context cancellation, took %v", elapsed)
	}
}

func TestExtractKeywords_Spanish(t *testing.T) {
	text := "Disfrutá de la mejor nieve, gastronomía de altura y eventos exclusivos en la Patagonia argentina."
	keywords := extractKeywords(text, 10)

	if len(keywords) == 0 {
		t.Fatal("Expected keywords from Spanish text")
	}
	joined := strings.Join(keywords, " ")
	for _, want := range []string{"nieve", "patagonia", "gastronomía", "altura"} {
		if !strings.Contains(strings.ToLower(joined), want) {
			t.Errorf("Keywords should contain %q, got: %s", want, joined)
		}
	}
}

func TestExtractKeywords_FiltersStopwords(t *testing.T) {
	text := "the quick brown fox jumps over the lazy dog"
	keywords := extractKeywords(text, 10)

	for _, kw := range keywords {
		if kw == "the" || kw == "over" {
			t.Errorf("Stopword %q should be filtered, got: %v", kw, keywords)
		}
	}
	joined := strings.Join(keywords, " ")
	for _, want := range []string{"quick", "brown", "fox", "jumps", "lazy"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Keywords should contain %q, got: %s", want, joined)
		}
	}
}

func TestExtractCompanyFromTitle(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Acme Corp | Leading Widget Solutions", "Acme Corp"},
		{"Cerro Bayo Ski Boutique", "Cerro Bayo Ski Boutique"},
		{"EmpresaX - Productos", "EmpresaX"},
		{"", ""},
		{"SoloNombre", "SoloNombre"},
	}
	for _, tc := range tests {
		got := extractCompanyFromTitle(tc.title)
		if got != tc.want {
			t.Errorf("extractCompanyFromTitle(%q) = %q, want %q", tc.title, got, tc.want)
		}
	}
}

func TestExtractCompanyFromCopyright(t *testing.T) {
	tests := []struct {
		body string
		want string
	}{
		{"© 2024 Cerro Bayo. All Rights Reserved.", "Cerro Bayo"},
		{"Copyright 2023 Acme Corp All Rights Reserved", "Acme Corp"},
		{"No copyright here", ""},
		{"©2024SingleWord", ""}, // no space between year and name — fails
		{"© 2024 EmpresaX", "EmpresaX"},
	}
	for _, tc := range tests {
		got := extractCompanyFromCopyright(tc.body)
		if got != tc.want {
			t.Errorf("extractCompanyFromCopyright(%q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}
