# Troubleshooting

Run `check-now plugin create ... --non-interactive` with all required fields, or use the wizard and read its short field descriptions. Use `--force` only when replacing the exact intended plugin folder.

If a plugin is absent from `check-now list`, verify `plugin.sh` exists, is executable, and responds to a metadata request whose ID matches its folder. `check-now doctor` reports the discovered set and cache path. Malformed output, stdout diagnostics, permissions, non-zero exits, and timeouts are rejected; diagnostics should be sent to stderr.

Run the generated `test.sh`, then `go test ./...`. Tests must use local fake commands and must not require network access.

## No output from `check-now shell`

`shell` reads the cache; it is not a synchronous refresh command. Run:

```sh
check-now refresh --force
check-now shell
check-now doctor
```

If the refresh is unknown or fails, inspect stderr from the foreground refresh. A stale valid result remains visible, while an initial failed refresh can legitimately leave the cache empty.

## Node reports no update

The Node plugin checks the installed executable and first tries to prove ownership by fnm or nvm. For fnm it uses `fnm current` and `fnm list-remote`; for nvm it requires the running Node path to be under `NVM_DIR` and uses `nvm ls-remote`. Without proven manager ownership it may still identify newer Node releases, but it returns `unknown` rather than guessing an install command. Check `command -v node`, `node --version`, `command -v fnm`, `command -v nvm`, and `NVM_DIR`.

Use `CHECK_NOW_NODE_RELEASE_URL` and the `CHECK_NOW_*_TARGET` variables only with local fixtures when reproducing parser behavior. They are not a replacement for real detection.

## Plugin output and discovery failures

Run a plugin directly with a metadata request to isolate it:

```sh
printf '%s\n' '{"protocol":1,"id":"node","mode":"metadata","timestamp":"2026-01-01T00:00:00Z"}' | plugins/node/plugin.sh
```

The command must emit exactly one JSON object on stdout. Human diagnostics belong on stderr. Then verify executable permissions (`chmod +x plugins/<id>/plugin.sh plugins/<id>/test.sh`), folder/metadata ID equality, and the configured roots in `CHECK_NOW_PLUGIN_DIRS`.

For an installed copy, confirm the executable-relative development path or `$CHECK_NOW_PREFIX/share/check-now/plugins` exists. Run `install.sh --build` or `make build` after changing source plugins; the host does not discover standalone legacy `check-now-*` files.

