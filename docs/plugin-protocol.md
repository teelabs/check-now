# Plugin protocol

Plugins read one JSON request from stdin and write one JSON object to stdout. Diagnostics belong on stderr. The protocol version is `1`.

The host starts a fresh process for each request and sends one newline-terminated object. A minimal request is:

```json
{"protocol":1,"id":"node","mode":"metadata","timestamp":"2026-01-01T00:00:00Z"}
```

The refresh form uses the same envelope and may include configuration and the last valid result in `prior`. A plugin must not depend on undocumented host environment state for protocol correctness.

`metadata` requests return `protocol`, `id`, `name`, `description`, `runtime`, `capabilities`, and optional `tool`, `source_url`, and `manager`. The host requires the metadata ID to equal the plugin folder ID.

`refresh` requests return `protocol`, `id`, `status`, `tool`, and RFC3339 `checked_at`. Status is `not_installed`, `up_to_date`, `update_available`, or `unknown`; optional fields include versions, manager, source URL, explanation, and structured `argv`.

An optional `updates` array can expose multiple policy choices. Each entry has `scope`, `track`, and `version`, plus optional `source_url`, `explanation`, and `argv`. For example, a Node plugin can report both the newest `24.x` release and the newest release overall. The terminal UI renders these entries as separate rows rather than exposing the machine status string.

`argv` is display-only. The host never executes it. Built-ins return `unknown` with an explanation when a newer release exists but the installation manager is not proven; they do not invent an update command.

## Result examples

Metadata with optional fields omitted:

```json
{"protocol":1,"id":"example","name":"Example","description":"Checks Example installations","runtime":"shell","capabilities":["refresh"]}
```

Conservative refresh result:

```json
{"protocol":1,"id":"example","status":"unknown","tool":"example","checked_at":"2026-01-01T00:00:00Z","explanation":"The installation manager could not be identified."}
```

An update result may include safe display guidance:

```json
{"protocol":1,"id":"node","status":"update_available","tool":"node","versions":["24.22.1"],"manager":"fnm","source_url":"https://nodejs.org","checked_at":"2026-01-01T00:00:00Z","updates":[{"scope":"same_major","track":"24.x","version":"24.22.1","argv":["fnm","install","24.22.1"]}]}
```

Only protocol JSON belongs on stdout. Do not pretty-print multiple objects, add progress messages, or put diagnostics in the response stream. Keep tests offline and use fake commands or local fixture endpoints; never execute `argv` in a plugin test.

