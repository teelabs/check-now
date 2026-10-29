# Troubleshooting

Run `check-now plugin create ... --non-interactive` with all required fields, or use the wizard and read its short field descriptions. Use `--force` only when replacing the exact intended plugin folder.

If a plugin is absent from `check-now list`, verify `plugin.sh` exists, is executable, and responds to a metadata request whose ID matches its folder. `check-now doctor` reports the discovered set and cache path. Malformed output, stdout diagnostics, permissions, non-zero exits, and timeouts are rejected; diagnostics should be sent to stderr.

Run the generated `test.sh`, then `go test ./...`. Tests must use local fake commands and must not require network access.

## No output from `check-now shell`

`shell` probes local versions synchronously but reads remote targets from cache; it is not a synchronous refresh command. It starts a non-forced background refresh only when target data is missing or stale. Run:

```sh
check-now refresh --force
check-now shell
check-now doctor
```

If the refresh is unknown or fails, inspect stderr from the foreground refresh. A valid previous target remains visible, while an initial failed refresh can legitimately leave the target cache empty. The current version still comes from the local probe on every shell invocation.

## Node reports no update

The Node plugin checks the installed executable and first tries to prove ownership by fnm or nvm. For fnm it uses `fnm current` and `fnm list-remote`; for nvm it requires the running Node path to be under `NVM_DIR` and uses `nvm ls-remote`. Without proven manager ownership it may still identify newer Node releases, but it returns `unknown` rather than guessing an install command. Check `command -v node`, `node --version`, `command -v fnm`, `command -v nvm`, and `NVM_DIR`.

Use `CHECK_NOW_NODE_RELEASE_URL` and the `CHECK_NOW_*_TARGET` variables only with local fixtures when reproducing parser behavior. They are not a replacement for real detection.

## Claude Code or Codex reports no update

The Claude Code checker recognizes the native version store below `~/.local/share/claude/versions` and global npm ownership. The Codex checker recognizes the standalone release store below `$CODEX_HOME/packages/standalone/releases`, global npm ownership, and a binary owned by Homebrew. When ownership is proven, refresh shows a documented copy-paste update command; otherwise it keeps the result `unknown` rather than guessing. For the standalone Codex installer, the documented update command is:

```sh
curl -fsSL https://chatgpt.com/codex/install.sh | sh
```

Check `command -v claude`, `command -v codex`, and their resolved paths with `readlink -f` or `realpath` when diagnosing an unrecognized installation.

## Plugin output and discovery failures

Run a plugin directly with a metadata request to isolate it:

```sh
printf '%s\n' '{"protocol":1,"id":"node","mode":"metadata","timestamp":"2026-01-01T00:00:00Z"}' | plugins/node/plugin.sh
```

The command must emit exactly one JSON object on stdout. Human diagnostics belong on stderr. Then verify executable permissions (`chmod +x plugins/<id>/plugin.sh plugins/<id>/test.sh`), folder/metadata ID equality, and the configured roots in `CHECK_NOW_PLUGIN_DIRS`.

For an installed copy, confirm the executable-relative development path or `$CHECK_NOW_PREFIX/share/check-now/plugins` exists. Run `install.sh --build` or `make build` after changing source plugins; the host does not discover standalone legacy `check-now-*` files.

