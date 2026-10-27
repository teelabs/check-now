# Changelog

All notable changes to this project are documented here.

## [Unreleased]

### Added

- Built-in checkers now resolve current releases from official Node.js, Bun, Deno, fnm, and Codex release sources.
- Node detection now uses proven fnm or nvm installation ownership when available.
- Deterministic offline fixtures cover release lookup, manager selection, protocol whitespace, and offline failures.
- Optional multi-target update choices and a readable terminal update table.
- Aligned ASCII table rendering with a spanning section header for terminal output.
- Centered grouped tool/source labels and removed repeated labels across version rows.
- Added odd-height grouped version rows so shared labels are vertically centered.

### Fixed

- Prevented false `up_to_date` results when release lookup fails.
- Hardened cache identity, RFC3339 validation, duplicate plugin-ID precedence, and stale refresh-lock recovery.
- Preserved the last valid cache when a refresh returns `unknown`.
