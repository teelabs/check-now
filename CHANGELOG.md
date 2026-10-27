# Changelog

All notable changes to this project are documented here.

## [Unreleased]

### Added

- The update table now shows the installed `Current` version separately from each semantic `Target`.
- Built-in checkers now resolve current releases from official Node.js, Bun, Deno, fnm, and Codex release sources.
- Added a conservative built-in Claude Code checker using `claude --version` and the official release feed.
- Node detection now uses proven fnm or nvm installation ownership when available.
- Deterministic offline fixtures cover release lookup, manager selection, protocol whitespace, and offline failures.
- Optional multi-target update choices and a readable terminal update table.
- Aligned ASCII table rendering with a spanning section header for terminal output.
- Centered grouped tool/source labels and removed repeated labels across version rows.
- Added odd-height grouped version rows so shared labels are vertically centered.

### Fixed

- Shell-triggered background refreshes now separate local current probes from cached remote target discovery.
- Bounded plugin stdout/stderr capture and serialized dismissal writes with refresh state.
- Rejected remote target fields from `current` responses so local probing cannot smuggle refresh behavior into shell startup.
- Prevented failed local probes from rendering cached targets without a confirmed current version.
- Bounded background refresh lifetime, reduced duplicate refresh work, sanitized terminal controls, and corrected prerelease ordering.
- Cleaned up descendant processes from timed-out or completed plugin requests on Unix.
- Prevented false `up_to_date` results when release lookup fails.
- Hardened cache identity, RFC3339 validation, duplicate plugin-ID precedence, and stale refresh-lock recovery.
- Preserved the last valid cache when a refresh returns `unknown`.
