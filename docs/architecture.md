# Architecture

## Runtime flow

The Go host discovers executable `plugin.sh` files in configured directories and in development/installed built-in directories. The parent folder is the canonical plugin ID. Discovery rejects non-directories, non-executable entrypoints, duplicate IDs after the first match, and metadata whose `id` does not match the folder name.

For every accepted plugin, the host first sends `{"protocol":1,"mode":"metadata",...}` and requires both `current` and `refresh` capabilities. `current` is a local-only probe; `refresh` owns remote target discovery and may receive an optional prior target result. The bounded subprocess runner captures stdout and stderr separately, rejects malformed or trailing JSON, validates timestamps and status values, and kills timed-out plugin processes.

Validated target results, current probe results, and dismissal controls are stored atomically below `$XDG_CACHE_HOME/check-now` (or `~/.cache/check-now`) in `targets/`, `current/`, and `control/`; old root `<id>.json` files are read as non-dismissed target entries. `refresh` runs in the foreground. `shell` probes current versions synchronously and starts one bounded non-forced background refresh only for missing/stale targets. A failed, malformed, timed-out, or offline refresh does not overwrite a valid target cache.

## Trust boundaries

Plugins are trusted local executables and therefore can inspect the local machine and make network requests. They own detection and release interpretation; the host does not duplicate checker logic. Plugin output is still untrusted data: returned `argv`, URLs, names, and explanations are validated as protocol data and displayed only.

The host never evaluates shell output, downloads plugins, installs tools, or executes returned `argv`. Update commands are copy-paste guidance only. A plugin must return `unknown` when it cannot prove installation ownership or construct a safe manager-specific command.

## Installation layout

The source layout is `plugins/<id>/plugin.sh`. Builds copy the complete plugin directories to `dist/plugins`; installation copies them to `$CHECK_NOW_PREFIX/share/check-now/plugins`. This preserves each plugin's README, AGENTS.md, and offline `test.sh` alongside its executable.

