// SmartWordlist generates contextual password wordlists for authorized security assessments.
//
// Usage:
//
//	smartwordlist <domain> [flags]
//
// Flags:
//
//	-o, --output          Output file path (default: stdout)
//	-m, --max             Maximum candidates to generate (0 = unlimited)
//	-v, --verbose         Verbose output
//	    --no-llm          Disable LLM-enhanced generation (rule-only mode)
//	-r, --rules           Path to mutation rules YAML file (default: defaults/rules.yaml)
//	    --json            JSON metadata output path (default: <output>.json, empty = no JSON)
//	    --model           Ollama LLM model name (default: qwen3:0.6b, env: SMARTWORDLIST_MODEL)
//	    --embedding-model Ollama embedding model name (default: nomic-embed-text, env: SMARTWORDLIST_EMBED_MODEL)
//	    --dry-run-ollama  Check Ollama health and model availability, then exit
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/Ivogomez03/smartwordlist/internal/cli"
	"github.com/Ivogomez03/smartwordlist/internal/export"
	"github.com/Ivogomez03/smartwordlist/internal/generation"
	"github.com/Ivogomez03/smartwordlist/internal/ollama"
	"github.com/Ivogomez03/smartwordlist/internal/plugin"
	"github.com/Ivogomez03/smartwordlist/internal/rag"
	"github.com/Ivogomez03/smartwordlist/internal/recon"
	"github.com/Ivogomez03/smartwordlist/internal/scoring"
	"github.com/Ivogomez03/smartwordlist/pkg/dict"
	"github.com/Ivogomez03/smartwordlist/pkg/types"
)

// defaultOllamaURL is the default Ollama server address.
const defaultOllamaURL = "http://localhost:11434"

// defaultOllamaModel is the LLM model used for candidate generation.
const defaultOllamaModel = "qwen3:0.6b"

// defaultEmbedModel is the embedding model for RAG vector search.
const defaultEmbedModel = "nomic-embed-text"

// pipelineTimeout is the maximum total pipeline duration.
const pipelineTimeout = 5 * time.Minute

// domainRegex is the validation pattern for domain names and IP addresses.
// It accepts bare domains (example.com), subdomains (www.example.com),
// and IP addresses (192.168.1.1) but rejects URLs with protocol, paths,
// ports, or other invalid characters.
var domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+|\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})$`)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, cli.Error(err.Error()))
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "smartwordlist <domain>",
	Short: "Generate contextual password wordlists for authorized security assessments",
	Long: cli.Banner() + "\nSmartWordlist combines reconnaissance, RAG, and local LLM generation to\n" +
		"produce targeted password wordlists for authorized penetration testing.\n\n" +
		"Example:\n  smartwordlist example.com --no-llm --max 1000 -o wordlist.txt",
	Args: cobra.ExactArgs(1),
	RunE: run,
}

func init() {
	rootCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
	rootCmd.Flags().IntP("max", "m", 2000, "Maximum candidates to generate")
	rootCmd.Flags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.Flags().Bool("no-llm", false, "Disable LLM-enhanced generation (rule-only mode)")
	rootCmd.Flags().StringP("rules", "r", "defaults/rules.yaml", "Path to mutation rules YAML file")
	rootCmd.Flags().String("json", "", "JSON metadata output path (default: <output>.json if --output set)")
	rootCmd.Flags().String("model", envOrDefault("SMARTWORDLIST_MODEL", defaultOllamaModel), "Ollama LLM model name")
	rootCmd.Flags().String("embedding-model", envOrDefault("SMARTWORDLIST_EMBED_MODEL", defaultEmbedModel), "Ollama embedding model name")
	rootCmd.Flags().Bool("dry-run-ollama", false, "Check Ollama health and model availability, then exit")
	rootCmd.Flags().Duration("ollama-timeout", 0, "Ollama HTTP client timeout (0 = default 300s, env: SMARTWORDLIST_OLLAMA_TIMEOUT)")
}

func run(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	domain := args[0]

	// ---- flag parsing ----

	output, _ := cmd.Flags().GetString("output")
	max, _ := cmd.Flags().GetInt("max")
	verbose, _ := cmd.Flags().GetBool("verbose")
	noLLM, _ := cmd.Flags().GetBool("no-llm")
	rulesPath, _ := cmd.Flags().GetString("rules")
	jsonPath, _ := cmd.Flags().GetString("json")
	modelName, _ := cmd.Flags().GetString("model")
	embedModel, _ := cmd.Flags().GetString("embedding-model")
	dryRunOllama, _ := cmd.Flags().GetBool("dry-run-ollama")

	ollamaTimeout, _ := cmd.Flags().GetDuration("ollama-timeout")
	if ollamaTimeout == 0 {
		if envVal := os.Getenv("SMARTWORDLIST_OLLAMA_TIMEOUT"); envVal != "" {
			if parsed, err := time.ParseDuration(envVal); err == nil {
				ollamaTimeout = parsed
			}
		}
	}

	// Auto-derive JSON path from output when not explicitly set.
	if jsonPath == "" && output != "" {
		jsonPath = output + ".json"
	}

	cfg := &types.Config{
		Domain:        domain,
		Output:        output,
		Max:           max,
		Verbose:       verbose,
		NoLLM:         noLLM,
		RulesPath:     rulesPath,
		Model:         modelName,
		EmbedModel:    embedModel,
		DryRunOllama:  dryRunOllama,
	}

	// ---- domain validation (W3) ----

	if !domainRegex.MatchString(domain) {
		fmt.Fprintln(os.Stderr, cli.Error(fmt.Sprintf("invalid domain format: %q — expected format like example.com or www.example.com", domain)))
		return fmt.Errorf("invalid domain: %s", domain)
	}

	// ---- banner + config summary ----

	fmt.Print(cli.Banner())
	fmt.Println(cli.Info(fmt.Sprintf("Target domain: %s", cfg.Domain)))
	if cfg.Output != "" {
		fmt.Println(cli.Info(fmt.Sprintf("Output file: %s", cfg.Output)))
	}
	if cfg.Max > 0 {
		fmt.Println(cli.Info(fmt.Sprintf("Max candidates: %d", cfg.Max)))
	}
	if cfg.NoLLM {
		fmt.Println(cli.Warning("LLM disabled — rule-only mode active"))
	}
	if cfg.Verbose {
		fmt.Println(cli.Info("Verbose mode enabled"))
		fmt.Println(cli.Info(fmt.Sprintf("LLM model: %s | Embed model: %s", cfg.Model, cfg.EmbedModel)))
	}
	fmt.Println(cli.Info(fmt.Sprintf("Rules file: %s", cfg.RulesPath)))

	if !noLLM {
		smallModels := map[string]bool{
			"qwen3:0.6b": true, "qwen3:0.5b": true, "tinyllama": true,
			"llama3.2:1b": true, "phi3:mini": true,
		}
		if smallModels[cfg.Model] {
			fmt.Println(cli.Warning(fmt.Sprintf("Model %q is very small — output quality may be poor. Consider using a 3B+ model for better results.", cfg.Model)))
		}
	}

	// ---- context ----

	ctx, cancel := context.WithTimeout(context.Background(), pipelineTimeout)
	defer cancel()

	// ---- Ollama client init ----

	ollamaClient := ollama.NewClient(defaultOllamaURL, ollamaTimeout)

	// ---- dry-run-ollama (W4) ----

	if cfg.DryRunOllama {
		return runDryRunOllama(ctx, ollamaClient, cfg)
	}

	// ---- Ollama health check ----

	llmMode := !noLLM

	if llmMode {
		if err := ollamaClient.Health(ctx); err != nil {
			fmt.Println(cli.Warning(fmt.Sprintf("Ollama not available: %v", err)))
			fmt.Println(cli.Warning("Falling back to rule-only mode"))
			llmMode = false
		} else {
			fmt.Println(cli.Success("Ollama connected — LLM-enhanced generation active"))
		}
	}

	// ---- progress indicator (W2) ----

	phaseCh := make(chan string, 4)
	doneCh := make(chan struct{})
	go runProgressIndicator(phaseCh, doneCh, verbose)

	// ---- load dictionaries + rules ----

	dicts, dictErr := dict.LoadDictionaries()
	if dictErr != nil {
		fmt.Println(cli.Warning(fmt.Sprintf("Failed to load dictionaries: %v", dictErr)))
	}

	rules, err := plugin.LoadRulesFile(rulesPath)
	if err != nil {
		// If the default rules file is missing (common with go install),
		// fall back to the embedded copy compiled into the binary.
		if rulesPath == "defaults/rules.yaml" {
			fmt.Println(cli.Warning("Default rules file not found — using embedded rules"))
			rules, err = plugin.LoadDefaultRules()
		}
		if err != nil {
			close(doneCh)
			return fmt.Errorf("load rules: %w", err)
		}
	}
	mutEngine := generation.NewMutationEngine(rules)

	// ---- channel-based pipeline (W5) ----
	//
	// Stages communicate through typed channels. Each stage runs in its own
	// goroutine and uses context for cancellation.

	reconResultCh := make(chan *types.ReconResult, 1)
	candidatesCh := make(chan []types.Candidate, 1)
	enrichedCh := make(chan enrichedResult, 1)
	errCh := make(chan error, 4)

	// Stage 1: Reconnaissance
	go func() {
		if verbose {
			phaseCh <- "Reconnaissance"
		}
		phaseStart := time.Now()
		collector := recon.NewReconCollector()
		result, err := collector.Collect(ctx, domain)
		if err != nil {
			errCh <- fmt.Errorf("reconnaissance: %w", err)
			close(reconResultCh)
			return
		}
		if verbose {
			fmt.Println(cli.Info(fmt.Sprintf("Recon completed in %v", time.Since(phaseStart))))
		}
		// Warn if recon returned no useful data — downstream stages need it.
		if result.Title == "" && result.Company == "" && len(result.Keywords) == 0 {
			fmt.Println(cli.Warning("Reconnaissance returned no useful data — the site may be blocking the request or require JavaScript. Wordlist will be generic."))
		}
		reconResultCh <- result
	}()

	// Stage 2: Generation (LLM or rule-based)
	go func() {
		reconResult, ok := <-reconResultCh
		if !ok {
			close(candidatesCh)
			return // recon failed
		}
		phaseCh <- "Generating candidates"

		var allCandidates []types.Candidate
		var sourcesUsed []string

		if llmMode {
			llmCandidates, llmSources, llmOK := runLLMPipeline(ctx, ollamaClient, reconResult, domain, cfg, verbose)
			if llmOK {
				allCandidates = append(allCandidates, llmCandidates...)
				sourcesUsed = append(sourcesUsed, llmSources...)
			} else {
				llmMode = false
			}
		}

		if !llmMode {
			ruleCandidates, ruleSources := runRulePipeline(reconResult, mutEngine, max, verbose)
			allCandidates = append(allCandidates, ruleCandidates...)
			sourcesUsed = append(sourcesUsed, ruleSources...)
		}

		candidatesCh <- allCandidates

		// Stage 3: Enrichment (mutation + optional dictionary combos).
		// In LLM mode, the model generates final passwords — no mutations.
		go func() {
			candidates := <-candidatesCh
			if llmMode {
				// LLM mode: use candidates directly, no enrichment.
				enrichedCh <- enrichedResult{
					candidates:     candidates,
					sources:        sourcesUsed,
					totalGenerated: len(candidates),
				}
				return
			}
			// Rule-only mode: apply mutations and combos.
			enriched, allSources, mutationCount, comboCount := enrichCandidates(
				candidates, reconResult, mutEngine, dicts, max, verbose,
			)
			// Merge sources from generation stage.
			for _, s := range sourcesUsed {
				allSources = append(allSources, s)
			}
			enrichedCh <- enrichedResult{
				candidates:     enriched,
				sources:        allSources,
				mutationCount:  mutationCount,
				comboCount:     comboCount,
				totalGenerated: len(candidates), // pre-enrichment count
			}
		}()
	}()

	// Wait for enriched candidates or error.
	var enriched enrichedResult
	select {
	case enriched = <-enrichedCh:
	case err := <-errCh:
		close(doneCh)
		return err
	case <-ctx.Done():
		close(doneCh)
		return ctx.Err()
	}

	// ---- Scoring + Dedup ----

	phaseCh <- "Scoring and exporting"

	// Filter obvious junk before scoring.
	filtered := make([]types.Candidate, 0, len(enriched.candidates))
	for _, c := range enriched.candidates {
		if !isJunkCandidate(c.Word) {
			filtered = append(filtered, c)
		}
	}
	if verbose && len(filtered) < len(enriched.candidates) {
		fmt.Println(cli.Info(fmt.Sprintf("Filtered %d junk candidates", len(enriched.candidates)-len(filtered))))
	}

	scorer := scoring.NewScorer()
	totalRaw := enriched.totalGenerated + enriched.mutationCount + enriched.comboCount
	scored := scorer.Score(filtered)
	deduped := scorer.Deduplicate(scored)

	// Stabilise sort after dedup.
	sort.SliceStable(deduped, func(i, j int) bool {
		if deduped[i].Score != deduped[j].Score {
			return deduped[i].Score > deduped[j].Score
		}
		return len(deduped[i].Word) > len(deduped[j].Word)
	})

	// Truncate to --max
	if max > 0 && len(deduped) > max {
		deduped = deduped[:max]
	}

	if verbose {
		fmt.Println(cli.Info(fmt.Sprintf("Scoring + dedup: %d → %d candidates", totalRaw, len(deduped))))
	}

	// ---- Export ----

	// Deduplicate sources
	sourceSet := make(map[string]bool)
	var uniqueSources []string
	for _, s := range enriched.sources {
		if !sourceSet[s] {
			sourceSet[s] = true
			uniqueSources = append(uniqueSources, s)
		}
	}

	stats := types.Stats{
		TotalCandidates: totalRaw,
		GenerationTime:  time.Since(startTime),
		SourcesUsed:     uniqueSources,
		MutationCounts: map[string]int{
			"mutated": enriched.mutationCount,
			"combos":  enriched.comboCount,
		},
	}

	exporter := export.NewExporter()

	var textWriter io.Writer = os.Stdout
	var textFile *os.File
	if output != "" {
		textFile, err = os.Create(output)
		if err != nil {
			close(doneCh)
			return fmt.Errorf("create output file %s: %w", output, err)
		}
		defer textFile.Close()
		textWriter = textFile
		phaseCh <- "Exporting"
	}

	if err := exporter.ExportText(deduped, textWriter); err != nil {
		close(doneCh)
		return fmt.Errorf("export text: %w", err)
	}

	// JSON output
	if jsonPath != "" {
		var jsonWriter io.Writer
		var jsonFile *os.File
		if jsonPath == "stdout" || jsonPath == "-" {
			jsonWriter = os.Stdout
			if output == "" {
				fmt.Fprintln(os.Stdout, "\n--- JSON ---")
			}
		} else {
			jsonFile, err = os.Create(jsonPath)
			if err != nil {
				close(doneCh)
				return fmt.Errorf("create JSON file %s: %w", jsonPath, err)
			}
			defer jsonFile.Close()
			jsonWriter = jsonFile
		}

		if err := exporter.ExportJSON(deduped, stats, jsonWriter); err != nil {
			close(doneCh)
			return fmt.Errorf("export JSON: %w", err)
		}
		if verbose {
			fmt.Println(cli.Info(fmt.Sprintf("JSON export: %s", jsonPath)))
		}
	}

	// Stop progress indicator.
	close(doneCh)

	// ---- Summary ----

	totalTime := time.Since(startTime)
	fmt.Println()
	fmt.Println(cli.Success(fmt.Sprintf("Done! Generated %d unique candidates (from %d raw)", len(deduped), totalRaw)))
	fmt.Println(cli.Info(fmt.Sprintf("Generation time: %v", stats.GenerationTime)))
	fmt.Println(cli.Info(fmt.Sprintf("Total time: %v", totalTime)))
	fmt.Println(cli.Info(fmt.Sprintf("Sources: %v", uniqueSources)))
	if output != "" {
		fmt.Println(cli.Success(fmt.Sprintf("Output written to: %s", output)))
	}
	if jsonPath != "" && jsonPath != "stdout" && jsonPath != "-" {
		fmt.Println(cli.Success(fmt.Sprintf("JSON metadata written to: %s", jsonPath)))
	}

	return nil
}

// ---------------------------------------------------------------------------
// enrichedResult carries the output of the enrichment stage (mutation + combos).
// ---------------------------------------------------------------------------

type enrichedResult struct {
	candidates     []types.Candidate
	sources        []string
	mutationCount  int
	comboCount     int
	totalGenerated int
}

// ---------------------------------------------------------------------------
// W4: dry-run-ollama implementation
// ---------------------------------------------------------------------------

func runDryRunOllama(ctx context.Context, client *ollama.Client, cfg *types.Config) error {
	fmt.Println(cli.Info("Dry-run: checking Ollama connectivity..."))

	if err := client.Health(ctx); err != nil {
		fmt.Println(cli.Error(fmt.Sprintf("Ollama health check FAILED: %v", err)))
		fmt.Println(cli.Warning("Make sure Ollama is running at " + defaultOllamaURL))
		return fmt.Errorf("ollama health check failed: %w", err)
	}
	fmt.Println(cli.Success("Ollama server: AVAILABLE"))

	// Check LLM model availability by attempting a ping via generate with
	// stream=false and an empty prompt. A 404 means model not found.
	fmt.Println(cli.Info(fmt.Sprintf("Checking model: %s", cfg.Model)))
	modelStatus := checkModel(ctx, client, cfg.Model)
	fmt.Println(modelStatus)

	fmt.Println(cli.Info(fmt.Sprintf("Checking embedding model: %s", cfg.EmbedModel)))
	embedStatus := checkModel(ctx, client, cfg.EmbedModel)
	fmt.Println(embedStatus)

	return nil
}

func checkModel(ctx context.Context, client *ollama.Client, model string) string {
	// Use a minimal prompt to test if the model exists.
	ch, err := client.Generate(ctx, model, "test", false)
	if err != nil {
		return cli.Warning(fmt.Sprintf("Model %s: UNAVAILABLE — %v", model, err))
	}
	// Drain the channel (single response for stream=false).
	<-ch
	return cli.Success(fmt.Sprintf("Model %s: AVAILABLE", model))
}

// ---------------------------------------------------------------------------
// W2: Progress indicator
// ---------------------------------------------------------------------------

// runProgressIndicator shows a spinner with the current pipeline phase.
// It runs in a goroutine and writes to stderr so it doesn't interfere
// with stdout output. When verbose is false, progress is suppressed.
func runProgressIndicator(phases <-chan string, done <-chan struct{}, verbose bool) {
	if !verbose {
		// Drain phases channel so senders don't block.
		for {
			select {
			case <-phases:
			case <-done:
				return
			}
		}
	}

	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	current := ""
	active := false

	for {
		select {
		case p := <-phases:
			if p != "" {
				current = p
				active = true
			}
		case <-ticker.C:
			if active {
				fmt.Fprintf(os.Stderr, "\r  %s %s...", spinner[i%len(spinner)], current)
				i++
			}
		case <-done:
			if active {
				fmt.Fprintf(os.Stderr, "\r  ✓ %s done\n", current)
			}
			return
		}
	}
}

// ---------------------------------------------------------------------------
// W5: Channel-based pipeline stages
// ---------------------------------------------------------------------------

// runLLMPipeline executes the full LLM path: chunk → embed → index → search → generate.
// It returns candidates, source labels, and whether it succeeded.
func runLLMPipeline(
	ctx context.Context,
	ollamaClient *ollama.Client,
	reconResult *types.ReconResult,
	domain string,
	cfg *types.Config,
	verbose bool,
) ([]types.Candidate, []string, bool) {
	// Chunk
	chunker := &rag.Chunker{}
	chunks := chunker.Chunk(reconResult)
	if len(chunks) == 0 {
		fmt.Println(cli.Warning("No chunks from recon — falling back to rule-only"))
		return nil, nil, false
	}

	// Extract texts for embedding
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}

	// Embed
	embedder := rag.NewOllamaEmbedder(ollamaClient, cfg.EmbedModel)
	embeddings, err := embedder.Embed(ctx, texts)
	if err != nil {
		fmt.Println(cli.Warning(fmt.Sprintf("Embedding failed: %v — falling back to rule-only", err)))
		return nil, nil, false
	}

	// Index into veclite
	cacheDir, err := vecliteCacheDir()
	if err != nil {
		fmt.Println(cli.Warning(fmt.Sprintf("Cache dir: %v — falling back to rule-only", err)))
		return nil, nil, false
	}

	vs, err := rag.NewVecliteStore(cacheDir, embedder.Dims())
	if err != nil {
		fmt.Println(cli.Warning(fmt.Sprintf("Veclite init: %v — falling back to rule-only", err)))
		return nil, nil, false
	}

	if err := vs.Index(ctx, domain, chunks, embeddings); err != nil {
		fmt.Println(cli.Warning(fmt.Sprintf("Veclite index: %v — falling back to rule-only", err)))
		return nil, nil, false
	}

	// Search: embed a query from recon context
	queryText := buildQueryText(reconResult)
	queryVecs, err := embedder.Embed(ctx, []string{queryText})
	if err != nil || len(queryVecs) == 0 {
		fmt.Println(cli.Warning(fmt.Sprintf("Query embedding failed: %v — falling back to rule-only", err)))
		return nil, nil, false
	}

	scoredChunks, err := vs.Search(ctx, domain, queryVecs[0], 5)
	if err != nil {
		fmt.Println(cli.Warning(fmt.Sprintf("RAG search failed: %v — using basic context", err)))
		// Don't fail — use chunks directly as context.
		scoredChunks = make([]types.ScoredChunk, len(chunks))
		for i, c := range chunks {
			scoredChunks[i] = types.ScoredChunk{Chunk: c, Score: 1.0}
		}
	}

	// Generate via LLM.
	// The first request to Ollama loads the model into RAM, which can take
	// 1-3 minutes on constrained hardware (MacBook Air, low RAM). The HTTP
	// client has a 300s timeout and the pipeline has a 5-min budget, so
	// there's plenty of time. Subsequent runs are fast since the model
	// stays loaded in Ollama's cache.
	if verbose {
		fmt.Println(cli.Info("Loading LLM model (first run may take 1-3 minutes)..."))
	}
	llmGen := generation.NewLLMGenerator(ollamaClient, cfg.Model)
	candidates, err := llmGen.Generate(ctx, scoredChunks, cfg.Max)
	if err != nil {
		fmt.Println(cli.Warning(fmt.Sprintf("LLM generation failed: %v — falling back to rule-only", err)))
		return nil, nil, false
	}

	if verbose {
		fmt.Println(cli.Success(fmt.Sprintf("LLM generated %d candidates", len(candidates))))
	}

	return candidates, []string{"llm"}, true
}

// runRulePipeline generates candidates using the rule-based engine.
func runRulePipeline(
	reconResult *types.ReconResult,
	mutEngine *generation.MutationEngine,
	max int,
	verbose bool,
) ([]types.Candidate, []string) {
	ruleGen := generation.NewRuleGenerator(reconResult, mutEngine)
	candidates, err := ruleGen.Generate(context.Background(), nil, max)
	if err != nil {
		fmt.Println(cli.Warning(fmt.Sprintf("Rule generation error: %v", err)))
		return nil, nil
	}

	if verbose {
		fmt.Println(cli.Success(fmt.Sprintf("Rule engine generated %d candidates", len(candidates))))
	}

	return candidates, []string{"rule-mutation", "rule-year"}
}

// enrichCandidates applies mutation and dictionary combination to the
// candidate list. It returns the enriched slice, all source labels,
// and counts for each enrichment step.
func enrichCandidates(
	baseCandidates []types.Candidate,
	reconResult *types.ReconResult,
	mutEngine *generation.MutationEngine,
	dicts map[string][]string,
	max int,
	verbose bool,
) ([]types.Candidate, []string, int, int) {
	allCandidates := make([]types.Candidate, len(baseCandidates))
	copy(allCandidates, baseCandidates)
	var sources []string

	// ---- Mutation ----
	mutStart := time.Now()
	mutatedCount := 0
	{
		seen := make(map[string]bool)
		for _, c := range allCandidates {
			seen[c.Word] = true
		}
		base := make([]types.Candidate, len(allCandidates))
		copy(base, allCandidates)
		for _, c := range base {
			for _, m := range mutEngine.Mutate(c.Word) {
				if !seen[m] {
					seen[m] = true
					mutatedCount++
					allCandidates = append(allCandidates, types.Candidate{
						Word:   m,
						Source: c.Source + "-mutated",
					})
				}
			}
		}
	}
	if verbose {
		fmt.Println(cli.Info(fmt.Sprintf("Mutations: +%d new variants in %v", mutatedCount, time.Since(mutStart))))
	}

	// ---- Dictionary combinations ----
	comboStart := time.Now()
	comboCount := 0
	{
		var dictWords []string
		for _, words := range dicts {
			dictWords = append(dictWords, words...)
		}
		// Cap dictionary words to avoid combinatorial explosion — the top
		// entries cover the most common patterns.
		if len(dictWords) > 50 {
			dictWords = dictWords[:50]
		}

		ctxWords := extractContextWords(reconResult)
		combos := generation.GenerateCombos(dictWords, ctxWords, mutEngine.Mutate)
		seen := make(map[string]bool)
		for _, c := range allCandidates {
			seen[c.Word] = true
		}
		for _, w := range combos {
			if !seen[w] {
				seen[w] = true
				comboCount++
				allCandidates = append(allCandidates, types.Candidate{
					Word:   w,
					Source: "combo",
				})
			}
		}
		sources = append(sources, "combo")
	}
	if verbose {
		fmt.Println(cli.Info(fmt.Sprintf("Combinations: +%d new variants in %v", comboCount, time.Since(comboStart))))
	}

	return allCandidates, sources, mutatedCount, comboCount
}

// extractContextWords builds a list of contextual words from the recon result
// for dictionary combination. Subdomains are split into their prefix parts
// (e.g. "www.example.com" → "www") and the apex domain is filtered out.
// Company names are split into individual words.
func extractContextWords(r *types.ReconResult) []string {
	if r == nil {
		return nil
	}
	var words []string
	// Split company name into individual words.
	for _, part := range strings.Fields(r.Company) {
		part = strings.ToLower(strings.TrimSpace(part))
		if len(part) > 2 && !isJunkWord(part) {
			words = append(words, part)
		}
	}
	// Technologies are NOT used as password base words — they're context
	// for the LLM, not material for mutation. Nobody uses "nginx" as a password.
	for _, kw := range r.Keywords {
		kw = strings.ToLower(strings.TrimSpace(kw))
		// Skip domain-like keywords — they produce junk passwords.
		if len(kw) > 2 && !isJunkWord(kw) && !strings.Contains(kw, ".") {
			words = append(words, kw)
		}
	}
	// Subdomains: only keep the prefix part, not the full FQDN.
	for _, sd := range r.Subdomains {
		prefix, _, _ := strings.Cut(sd, ".")
		prefix = strings.ToLower(strings.TrimSpace(prefix))
		if prefix != "" && prefix != "www" && len(prefix) > 2 && !isJunkWord(prefix) {
			words = append(words, prefix)
		}
	}
	return dedupeStrings(words)
}

// isJunkWord returns true for words that are too generic to be useful
// as password base words.
func isJunkWord(w string) bool {
	junk := map[string]bool{
		"the": true, "and": true, "for": true, "new": true, "all": true,
		"our": true, "its": true, "has": true, "are": true, "was": true,
		"can": true, "not": true, "you": true, "your": true, "from": true,
		"that": true, "this": true, "with": true, "have": true, "been": true,
		"will": true, "more": true, "page": true, "home": true, "site": true,
		"need": true, "run": true, "app": true, "api": true, "use": true,
		"get": true, "one": true, "two": true, "see": true, "now": true,
		"com": true, "org": true, "net": true, "www": true,
		"enable": true, "javascript": true, "cookie": true, "function": true,
	}
	return junk[strings.ToLower(strings.TrimSpace(w))]
}

// cleanTechWord strips version numbers and common prefixes from technology names.
func cleanTechWord(t string) string {
	t = strings.TrimSpace(t)
	// Strip version numbers: "nginx/1.24" → "nginx", "jquery/3.7" → "jquery"
	if idx := strings.Index(t, "/"); idx > 0 {
		t = t[:idx]
	}
	// Split compound tech names: "Google Analytics" → "google", "analytics"
	// Return the first meaningful word.
	t = strings.SplitN(t, " ", 2)[0]
	t = strings.SplitN(t, "/", 2)[0]
	return strings.ToLower(strings.TrimSpace(t))
}

// buildQueryText creates a search query from the most relevant recon fields.
func buildQueryText(r *types.ReconResult) string {
	parts := []string{}
	if r.Company != "" {
		parts = append(parts, "Company: "+r.Company)
	}
	if r.Title != "" {
		parts = append(parts, "Title: "+r.Title)
	}
	if len(r.Keywords) > 0 {
		kw := r.Keywords
		if len(kw) > 10 {
			kw = kw[:10]
		}
		parts = append(parts, "Keywords: "+joinStrings(kw, ", "))
	}
	parts = append(parts, "wordlist generation password candidates")
	return joinStrings(parts, " | ")
}

// vecliteCacheDir returns the XDG-compatible cache directory.
func vecliteCacheDir() (string, error) {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}
		cacheHome = home + "/.cache"
	}
	return cacheHome + "/smartwordlist", nil
}

// envOrDefault returns the env var value if set, or the fallback.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func dedupeStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s == "" {
			continue
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// isJunkCandidate returns true for candidates that are obviously not
// real passwords. It filters whitespace, very short words, purely numeric
// strings, and strings with no letters at all.
func isJunkCandidate(w string) bool {
	if strings.ContainsAny(w, " \t\n\r") {
		return true
	}
	// Filter candidates shorter than 4 chars.
	if len(w) < 4 {
		return true
	}
	// Filter purely numeric candidates.
	if isAllDigitsStr(w) {
		return true
	}
	// Filter candidates that are only special chars.
	hasLetter := false
	for _, r := range w {
		if unicode.IsLetter(r) {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return true
	}
	return false
}

// isAllDigitsStr returns true when every rune in s is a digit.
func isAllDigitsStr(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
