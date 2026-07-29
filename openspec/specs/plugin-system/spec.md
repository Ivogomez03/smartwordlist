# plugin-system Specification

## Purpose

YAML/TOML rule file loading with validation, Go plugin interface for custom collectors and providers, and plugin panic isolation.

## Requirements

### Requirement: Rule File Loading

The system MUST load YAML or TOML rule configuration files containing mutation rules, regex patterns, and word combination templates.

#### Scenario: Valid YAML rules file

- GIVEN a `rules.yaml` with leet substitutions and suffix patterns
- WHEN the plugin system loads it
- THEN all rules are available to the mutation engine

### Requirement: Rule Validation

The system MUST validate rule files at load time and report syntax or semantic errors.

#### Scenario: Invalid YAML syntax

- GIVEN a rules file with malformed YAML
- WHEN loading executes
- THEN a specific error message with line number is displayed and execution halts

#### Scenario: Valid syntax but unknown keys

- GIVEN a rules file with an unrecognized top-level key
- WHEN validation runs
- THEN a warning is emitted and the unknown section is ignored

### Requirement: Go Plugin Interface

The system MUST provide a Go plugin interface allowing users to register custom reconnaissance collectors, embedding providers, and candidate generators.

#### Scenario: Custom recon collector

- GIVEN a compiled `.so` plugin implementing `ReconCollector`
- WHEN the plugin is loaded at startup
- THEN its `Collect(domain)` method is called during the recon phase

### Requirement: Plugin Panic Isolation

The system MUST isolate plugin panics so one failing plugin does not crash the process.

#### Scenario: Faulty plugin

- GIVEN a plugin that panics during `Collect()`
- WHEN the panic occurs
- THEN the error is logged and the pipeline continues with remaining plugins

### Requirement: Default Rules File

The system SHALL embed a default rules file in the binary.

#### Scenario: No --rules flag provided

- GIVEN no custom rules file is specified
- WHEN the tool starts
- THEN the embedded default rules are loaded

### Requirement: Custom Rules Path

The system SHALL support a `--rules` flag for custom file paths.

#### Scenario: Custom rules specified

- GIVEN `--rules /path/to/custom.yaml`
- WHEN the tool starts
- THEN the specified file is loaded instead of defaults
