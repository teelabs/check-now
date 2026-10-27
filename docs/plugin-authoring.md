# Plugin authoring

See [plugin protocol](plugin-protocol.md) for the complete metadata, current, and refresh contracts. Use `check-now plugin create <id>` to generate the offline-safe starting bundle.

Protocol version `1` requests contain `protocol`, `id`, `mode`, `timestamp`, configuration, and optionally `prior`. Modes are `metadata`, `current`, and `refresh`. Results must contain `protocol: 1`, `id`, `status`, `tool`, and an RFC3339 `checked_at`. Status is one of `not_installed`, `up_to_date`, `update_available`, or `unknown`.

For `update_available`, include versions and `argv` only when the installation manager is confidently known. Include an official `source_url` and explanation when a command cannot be safely determined. Logs belong on stderr; stdout contains exactly one JSON result.

Plugins are trusted local code. There is no remote installation mechanism, and `argv` is display-only. The canonical ID is the containing directory name and `plugin.sh` is the only executable entrypoint.

Built-in checkers must resolve targets from their documented official source. A failed or malformed release lookup is `unknown`, never `up_to_date`. Node additionally uses `fnm list-remote` or `nvm ls-remote` when it can prove the current Node belongs to that manager; otherwise it falls back to the official Node distribution index without inventing an update command. Claude Code uses `claude --version` and the official `anthropics/claude-code` release feed, and does not invent an update command when installation ownership is unclear.
