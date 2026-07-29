# reconnaissance Specification

## Purpose

HTTP scraping via colly: HTML parsing, title/keyword/company extraction, tech detection, DNS/subdomain enumeration, robots.txt/sitemap.xml parsing, email harvesting, and structured JSON output.

## Requirements

### Requirement: Page Metadata Extraction

The system MUST fetch target domain HTML and extract: page title, meta description, company/organization name, and visible keywords.

#### Scenario: Successful page fetch

- GIVEN `https://example.com` is reachable and returns HTML
- WHEN the recon phase executes
- THEN title, meta description, and keywords are extracted into structured fields

#### Scenario: Unreachable domain

- GIVEN a domain that resolves but returns HTTP 5xx
- WHEN the fetch fails after retries
- THEN the error is logged and the pipeline continues with partial data

### Requirement: Tech Stack Detection

The system MUST attempt tech stack detection via HTTP headers, meta tags, and JS library references.

#### Scenario: Detectable tech signals

- GIVEN the page includes `X-Powered-By: Express` header
- WHEN tech detection runs
- THEN `Express` is recorded in the tech stack list

### Requirement: Subdomain Enumeration

The system MUST enumerate subdomains via DNS resolution of common prefixes and certificate transparency logs.

#### Scenario: DNS enumeration

- GIVEN a target domain with common subdomains (www, mail, api)
- WHEN the enumeration phase runs
- THEN resolved subdomains are added to the output JSON

### Requirement: robots.txt and Sitemap Parsing

The system MUST fetch and parse `robots.txt` and `sitemap.xml` when they exist.

#### Scenario: robots.txt found

- GIVEN `example.com/robots.txt` returns 200
- WHEN parsing executes
- THEN disallowed and allowed paths are extracted

### Requirement: Email Harvesting

The system MUST extract email addresses from public pages using regex.

#### Scenario: Emails found in page body

- GIVEN the HTML contains `contact@example.com` in visible text
- WHEN email extraction runs
- THEN the address is collected deduplicated

### Requirement: Structured JSON Output

The recon phase SHALL output all collected data as structured JSON.

#### Scenario: Recon complete

- GIVEN all recon sub-phases have finished
- WHEN the pipeline serializes results
- THEN a valid JSON object with keys for each domain is produced
