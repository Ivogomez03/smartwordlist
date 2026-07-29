package recon

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

// scrapeResult holds the structured output of a single HTML scrape pass.
type scrapeResult struct {
	Title        string
	Company      string
	Keywords     []string
	Technologies []string
	Emails       []string
}

// emailRegex matches RFC 5322-ish email addresses found in page text.
// It is intentionally simpler than the full spec to avoid noise.
var emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

// techScriptPatterns maps substrings found in <script src> attributes to a
// human-readable technology label.
var techScriptPatterns = map[string]string{
	"react":            "React",
	"react-dom":        "React",
	"vue":              "Vue.js",
	"jquery":           "jQuery",
	"bootstrap":        "Bootstrap",
	"angular":          "Angular",
	"next":             "Next.js",
	"nuxt":             "Nuxt.js",
	"svelte":           "Svelte",
	"alpine":           "Alpine.js",
	"htmx":             "HTMX",
	"lodash":           "Lodash",
	"moment":           "Moment.js",
	"d3":               "D3.js",
	"three":            "Three.js",
	"chart":            "Chart.js",
	"tailwind":         "Tailwind CSS",
	"font-awesome":     "Font Awesome",
	"google-analytics": "Google Analytics",
	"gtag":             "Google Analytics",
	"googletagmanager": "Google Tag Manager",
	"gtm":              "Google Tag Manager",
}

// headerTechKeys maps HTTP response header names to the technology label
// recorded when the header is present.
var headerTechKeys = map[string]string{
	"x-powered-by": "X-Powered-By",
	"server":       "Server",
	"x-generator":  "X-Generator",
}

// cookieTechPatterns maps cookie-name substrings to technology labels.
var cookieTechPatterns = map[string]string{
	"laravel_session":  "Laravel",
	"wordpress":        "WordPress",
	"wp-":              "WordPress",
	"PHPSESSID":        "PHP",
	"JSESSIONID":       "Java",
	"ASP.NET_SessionId": "ASP.NET",
	"cfduid":           "Cloudflare",
	"__cf":             "Cloudflare",
}

// stopWords is a compact set of common English words filtered during
// keyword extraction.
var stopWords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "but": {}, "in": {},
	"on": {}, "at": {}, "to": {}, "for": {}, "of": {}, "with": {}, "by": {},
	"from": {}, "is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {},
	"being": {}, "have": {}, "has": {}, "had": {}, "do": {}, "does": {}, "did": {},
	"will": {}, "would": {}, "could": {}, "should": {}, "may": {}, "might": {},
	"can": {}, "shall": {}, "this": {}, "that": {}, "these": {}, "those": {},
	"it": {}, "its": {}, "we": {}, "you": {}, "they": {}, "he": {}, "she": {},
	"not": {}, "no": {}, "all": {}, "each": {}, "every": {}, "both": {},
	"few": {}, "more": {}, "most": {}, "other": {}, "some": {}, "such": {},
	"only": {}, "own": {}, "same": {}, "so": {}, "than": {}, "too": {},
	"very": {}, "just": {}, "about": {}, "above": {}, "after": {}, "again": {},
	"against": {}, "between": {}, "into": {}, "through": {}, "during": {},
	"before": {}, "below": {}, "up": {}, "down": {}, "out": {}, "off": {},
	"over": {}, "under": {}, "here": {}, "there": {}, "when": {}, "where": {},
	"why": {}, "how": {}, "what": {}, "which": {}, "who": {}, "whom": {},
	"then": {}, "now": {}, "also": {}, "if": {}, "else": {}, "i": {},
	"me": {}, "my": {}, "myself": {}, "our": {}, "ours": {}, "your": {},
	"yours": {}, "his": {}, "her": {}, "hers": {}, "itself": {},
	"themselves": {}, "am": {}, "hadn": {}, "hasn": {}, "haven": {},
	"isn": {}, "aren": {}, "wasn": {}, "weren": {}, "don": {}, "doesn": {},
	"didn": {}, "won": {}, "wouldn": {}, "shouldn": {}, "couldn": {},
	"mustn": {}, "needn": {}, "mightn": {}, "shan": {},
	"further": {}, "once": {}, "get": {}, "got": {}, "one": {}, "two": {},
	"three": {}, "four": {}, "five": {}, "any": {}, "make": {}, "made": {},
	"see": {}, "use": {}, "used": {}, "using": {}, "know": {}, "take": {},
	"like": {}, "well": {}, "back": {}, "still": {}, "even": {}, "much": {},
	"way": {}, "new": {}, "first": {}, "last": {}, "many": {}, "good": {},
	"great": {}, "work": {}, "year": {}, "years": {}, "time": {}, "day": {},
	"days": {}, "people": {}, "find": {}, "found": {}, "give": {}, "given": {},
	"come": {}, "came": {}, "go": {}, "went": {}, "going": {}, "page": {},
	"site": {}, "website": {}, "http": {}, "https": {}, "www": {},
	"com": {}, "org": {}, "net": {}, "html": {}, "css": {}, "js": {},
	"copyright": {}, "rights": {}, "reserved": {}, "privacy": {}, "policy": {},
	"terms": {}, "conditions": {}, "contact": {}, "home": {}, "help": {},
}

// titleCompanySeps defines separator characters that commonly split a
// company name from a page section hint in <title> text.
var titleCompanySeps = []string{" | ", " - ", " — ", " · ", " :: ", " : "}

// scrapeTransport is the http.RoundTripper used by colly collectors created
// by scrapeHTML. Tests may override it to inject httptest transports or
// disable TLS verification.
var scrapeTransport http.RoundTripper

// scrapeHTML fetches the domain's homepage with colly and extracts
// title, company name, keywords, detected technologies, and email addresses.
func scrapeHTML(ctx context.Context, domain string) (*scrapeResult, error) {
	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"),
	)
	if scrapeTransport != nil {
		c.WithTransport(scrapeTransport)
	} else {
		// Force HTTP/1.1 to avoid Go's TLS fingerprint being detected
		// by WAFs (Cloudflare, etc.) that block Go's HTTP/2 handshake.
		c.WithTransport(&http.Transport{
			ForceAttemptHTTP2:     false,
			TLSHandshakeTimeout:   10 * time.Second,
			DisableKeepAlives:     false,
			MaxIdleConns:          5,
			IdleConnTimeout:       30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		})
	}

	result := &scrapeResult{}
	var bodyBuilder strings.Builder
	var rawBodyBuilder strings.Builder

	// Track whether the request actually succeeded. colly's Visit returns
	// nil when OnError is set — even on failures — so we must capture the
	// error and response status ourselves.
	var visitErr error
	var responseReceived bool
	var statusCode int
	var htmlCallbacksFired int

	// ---- HTML callbacks ----

	c.OnHTML("title", func(e *colly.HTMLElement) {
		htmlCallbacksFired++
		result.Title = strings.TrimSpace(e.Text)
	})

	c.OnHTML("meta[name=description]", func(e *colly.HTMLElement) {
		htmlCallbacksFired++
		bodyBuilder.WriteString(" " + e.Attr("content"))
	})

	c.OnHTML("meta[name=author]", func(e *colly.HTMLElement) {
		htmlCallbacksFired++
		if result.Company == "" {
			result.Company = strings.TrimSpace(e.Attr("content"))
		}
	})

	c.OnHTML("meta[property='og:site_name']", func(e *colly.HTMLElement) {
		htmlCallbacksFired++
		if result.Company == "" {
			result.Company = strings.TrimSpace(e.Attr("content"))
		}
	})

	// og:title as title fallback
	c.OnHTML("meta[property='og:title']", func(e *colly.HTMLElement) {
		htmlCallbacksFired++
		if result.Title == "" {
			result.Title = strings.TrimSpace(e.Attr("content"))
		}
	})

	c.OnHTML("body", func(e *colly.HTMLElement) {
		htmlCallbacksFired++
		bodyBuilder.WriteString(" " + e.Text)
	})

	c.OnHTML("meta[name=generator]", func(e *colly.HTMLElement) {
		htmlCallbacksFired++
		if tech := strings.TrimSpace(e.Attr("content")); tech != "" {
			result.Technologies = append(result.Technologies, tech)
		}
	})

	c.OnHTML("script[src]", func(e *colly.HTMLElement) {
		htmlCallbacksFired++
		src := strings.ToLower(e.Attr("src"))
		for pattern, label := range techScriptPatterns {
			if strings.Contains(src, pattern) {
				result.Technologies = append(result.Technologies, label)
			}
		}
	})

	// ---- response callback ----

	c.OnResponse(func(r *colly.Response) {
		responseReceived = true
		statusCode = r.StatusCode
		rawBodyBuilder.WriteString(string(r.Body))

		// Diagnostic: log what we received so the user can debug.
		fmt.Fprintf(os.Stderr, "[recon] HTTP %d | Content-Type: %s | Body: %d bytes | URL: %s\n",
			r.StatusCode,
			r.Headers.Get("Content-Type"),
			len(r.Body),
			r.Request.URL.String(),
		)

		// HTTP header-based tech detection
		for headerKey, label := range headerTechKeys {
			if val := r.Headers.Get(headerKey); val != "" {
				result.Technologies = append(result.Technologies, label+": "+val)
			}
		}

		// Cookie-based tech detection
		cookies := r.Headers.Values("Set-Cookie")
		for _, cookie := range cookies {
			lowerCookie := strings.ToLower(cookie)
			for pattern, label := range cookieTechPatterns {
				if strings.Contains(lowerCookie, strings.ToLower(pattern)) {
					result.Technologies = append(result.Technologies, label)
				}
			}
		}

		// Extract emails from raw HTML body
		result.Emails = append(result.Emails, emailRegex.FindAllString(string(r.Body), -1)...)
	})

	// ---- error callback ----
	// Must capture the error explicitly because colly suppresses it from
	// Visit() when any OnError handler is registered.

	c.OnError(func(r *colly.Response, err error) {
		visitErr = err
		if r != nil {
			statusCode = r.StatusCode
		}
		// Diagnostic so the user sees what happened.
		fmt.Fprintf(os.Stderr, "[recon] error: %v (HTTP %d)\n", err, statusCode)
	})

	// ---- visit ----

	url := "https://" + domain
	cVisitErr := c.Visit(url)

	// Prefer the status-aware error when available; colly's Visit error on
	// HTTP failures is just "Internal Server Error" without the code.
	if statusCode >= 300 {
		cVisitErr = fmt.Errorf("HTTP %d", statusCode)
	}
	if cVisitErr != nil {
		return nil, fmt.Errorf("scrape %s: %w", domain, cVisitErr)
	}

	// Surface OnError-captured errors (connection refused, TLS, DNS).
	if visitErr != nil {
		return nil, fmt.Errorf("scrape %s: %w", domain, visitErr)
	}

	// Defensive: if we never received a response, something went wrong.
	if !responseReceived {
		return nil, fmt.Errorf("scrape %s: no response received", domain)
	}

	// ---- post-processing (callbacks already fired) ----

	// Keyword extraction from visible text
	result.Keywords = extractKeywords(bodyBuilder.String(), 30)

	// Company name heuristics (only if og:site_name was absent)
	if result.Company == "" {
		result.Company = extractCompanyFromTitle(result.Title)
	}
	if result.Company == "" {
		result.Company = extractCompanyFromCopyright(rawBodyBuilder.String())
	}

	// Deduplicate
	result.Technologies = deduplicateStrings(result.Technologies)
	result.Keywords = deduplicateStrings(result.Keywords)
	result.Emails = deduplicateStrings(result.Emails)

	// Diagnostic: report what was extracted.
	fmt.Fprintf(os.Stderr, "[recon] HTML callbacks: %d | Title: %q | Company: %q | Keywords: %d | Techs: %d | Emails: %d\n",
		htmlCallbacksFired, result.Title, result.Company,
		len(result.Keywords), len(result.Technologies), len(result.Emails))

	return result, nil
}

// extractKeywords tokenises visible page text, strips stopwords, and returns
// the n most frequent words longer than 2 characters.
func extractKeywords(text string, n int) []string {
	words := tokenize(text)
	freq := make(map[string]int, len(words))

	for _, w := range words {
		lower := strings.ToLower(w)
		if len(lower) <= 2 {
			continue
		}
		if _, stop := stopWords[lower]; stop {
			continue
		}
		// Skip purely numeric tokens
		if isNumeric(lower) {
			continue
		}
		freq[lower]++
	}

	type kv struct {
		word  string
		count int
	}

	sorted := make([]kv, 0, len(freq))
	for w, c := range freq {
		sorted = append(sorted, kv{w, c})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	if n > len(sorted) {
		n = len(sorted)
	}

	result := make([]string, 0, n)
	for i := 0; i < n; i++ {
		result = append(result, sorted[i].word)
	}

	return result
}

// tokenize splits text on non-letter boundaries and returns clean tokens.
func tokenize(text string) []string {
	// Normalise whitespace and split on non-letter sequences.
	re := regexp.MustCompile(`[^a-zA-ZáéíóúüñÁÉÍÓÚÜÑ0-9]+`)
	parts := re.Split(text, -1)

	tokens := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			tokens = append(tokens, t)
		}
	}
	return tokens
}

// isNumeric reports whether s contains only digit characters.
func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// extractCompanyFromTitle attempts to pull a company name from the first
// segment of a <title> that uses common separator characters.
func extractCompanyFromTitle(title string) string {
	if title == "" {
		return ""
	}

	for _, sep := range titleCompanySeps {
		if idx := strings.Index(title, sep); idx > 0 {
			return strings.TrimSpace(title[:idx])
		}
	}
	// If no separator is found, return the whole title as the company hint.
	return strings.TrimSpace(title)
}

// extractCompanyFromCopyright scans raw HTML body text for copyright notices
// and extracts the organisation name. Both "© 2024 Name" and "Copyright 2024
// Name" forms are recognised. A year (or year range) is required between the
// marker and the company name to reduce false positives.
func extractCompanyFromCopyright(body string) string {
	crPattern := regexp.MustCompile(
		`(?i)(?:©|copyright)\s*` + // marker
			`\d{4}` + // year (required — avoids matching "no copyright here")
			`(?:\s*[-–—]\s*\d{4})?` + // optional year range
			`\s+` +
			`([A-ZÁÉÍÓÚÜÑ][A-Za-zÁÉÍÓÚÜÑáéíóúüñ0-9\s,&.]+?)` + // company name
			`(?:\s*(?:All\s+Rights\s+Reserved|\.|$))`,
	)
	matches := crPattern.FindStringSubmatch(body)
	if len(matches) >= 2 {
		name := strings.TrimSpace(matches[1])
		if len(name) > 2 {
			return name
		}
	}
	return ""
}

// deduplicateStrings removes duplicate entries from a slice, preserving
// the order of first occurrence.
func deduplicateStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(s))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}
