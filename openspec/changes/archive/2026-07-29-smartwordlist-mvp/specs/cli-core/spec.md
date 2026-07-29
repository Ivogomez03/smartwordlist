# cli-core Specification

## Purpose

CLI framework providing Cobra commands, argument parsing, Charm-stack colored output (Lip Gloss), progress bars (Bubble Tea), and banner display — sqlmap-style interface.

## Requirements

### Requirement: CLI Entry Point

The CLI MUST accept a required `<domain>` positional argument and optional flags: `--output`, `--max`, `--verbose`, `--no-llm`.

#### Scenario: Valid domain, minimal flags

- GIVEN the binary is invoked as `smartwordlist example.com`
- WHEN execution starts
- THEN the pipeline begins recon against example.com with default settings

#### Scenario: Invalid domain format

- GIVEN the command `smartwordlist not_a_domain`
- WHEN the argument is parsed
- THEN the CLI MUST display a red error message and exit with code 1

### Requirement: Colored Terminal Output

The CLI SHALL render banners, status messages, and errors using Lip Gloss styles.

#### Scenario: Startup banner

- GIVEN any valid invocation
- WHEN the command starts
- THEN a stylized banner with the tool name and version is displayed

### Requirement: Progress Bars

Progress bars MUST appear during recon and generation phases.

#### Scenario: Recon in progress

- GIVEN a recon phase is running
- WHEN sub-steps complete (e.g., HTML fetched, subdomains enumerated)
- THEN a Bubble Tea progress bar reflects incremental completion

### Requirement: Help and Version

`--help` SHALL display usage; `--version` SHALL print the version.

#### Scenario: Help flag

- GIVEN `smartwordlist --help`
- WHEN executed
- THEN command usage, flags, and examples are printed
