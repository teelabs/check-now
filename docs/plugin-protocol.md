# Plugin protocol

Plugins read one JSON request from stdin and write one JSON object to stdout. Diagnostics belong on stderr. The current protocol version is `2`; protocol v1 adapters must be updated before the host will accept them.

The host starts a fresh process for each request and sends one newline-terminated object:

```json
{"protocol":2,"id":"node","mode":"metadata","timestamp":"2026-01-01T00:00:00Z"}
```

The refresh form uses the same envelope and may include configuration and the last valid result in `prior`. A plugin must not depend on undocumented host environment state for protocol correctness.

`metadata` returns `protocol`, `id`, `name`, `description`, `runtime`, and `capabilities`, with optional `tool`, `source_url`, and `manager`. Every adapter must advertise `current`, `refresh`, and `health`; the host requires the metadata ID to equal the plugin folder ID.

`health` performs offline checks of local adapter configuration and prerequisites. It must not contact update sources, change configuration, or include arbitrary environment values or secrets in its output. Doctor replaces literal non-empty environment values in plugin IDs and health codes, summaries, and repair text before display. Very short values can make more of a message unreadable; adapters must still omit values because derived or transformed values cannot be reliably detected. The response has `protocol`, `id`, RFC3339 `checked_at`, and a non-empty `checks` array with at most 64 entries. Each check has a stable `code`, `status` (`ok`, `warning`, or `error`), and short `summary`, with optional `repair` guidance:

```json
{"protocol":2,"id":"example","checked_at":"2026-01-01T00:00:00Z","checks":[{"code":"command.available","status":"warning","summary":"The local command was not found on PATH.","repair":"Install the tool or correct PATH."}]}
```

`current` returns only local information: installed version, manager/installation kind when known, status, and `checked_at`. It must not call network APIs. `refresh` discovers remote targets and may return target choices; the host combines them with the live current result before rendering.

`refresh` returns `protocol`, `id`, `status`, `tool`, and RFC3339 `checked_at`. Status is `not_installed`, `up_to_date`, `update_available`, or `unknown`; optional fields include versions, manager, source URL, explanation, and structured `argv`.

An optional `updates` array can expose multiple policy choices. Each entry has `scope`, `track`, and `version`, plus optional `source_url`, `explanation`, and `argv`. The parent result's `current_version` is the installed version shown in the terminal UI's `Current` column. For example, a Node plugin can report both the newest `24.x` release and the newest release overall. The terminal UI renders these entries as separate rows under the `Target` column rather than exposing the machine status string.

`argv` is display-only. The host never executes it. Built-ins return `unknown` with an explanation when a newer release exists but the installation manager is not proven; they do not invent an update command.

## Result examples

Metadata with all required capabilities:

```json
{"protocol":2,"id":"example","name":"Example","description":"Checks Example installations","runtime":"shell","capabilities":["current","refresh","health"]}
```

Conservative refresh result:

```json
{"protocol":2,"id":"example","status":"unknown","tool":"example","checked_at":"2026-01-01T00:00:00Z","explanation":"The installation manager could not be identified."}
```

An update result may include safe display guidance:

```json
{"protocol":2,"id":"node","status":"update_available","tool":"node","current_version":"24.20.0","manager":"fnm","source_url":"https://nodejs.org","checked_at":"2026-01-01T00:00:00Z","updates":[{"scope":"same_major","track":"24.x","version":"24.22.1","argv":["fnm","install","24.22.1"]}]}
```

Only protocol JSON belongs on stdout. Do not pretty-print multiple objects, add progress messages, or put diagnostics in the response stream. Keep tests offline and use fake commands or local fixture endpoints; never execute `argv` in a plugin test.

