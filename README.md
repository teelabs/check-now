# check-now

`check-now` reports cached developer-tool updates and prints copy-pasteable commands without executing them. It is a small Go host with local executable plugins: discovery, protocol validation, caching, locking, and terminal presentation live in the host; checker logic lives in each plugin's `plugin.sh`.

The default workflow is deliberately conservative:

1. A plugin detects the installed tool and queries its documented official source.
2. The plugin returns a fixed JSON result, including safe display-only `argv` when the installation manager is proven.
3. `check-now refresh` validates and caches the result.
4. `check-now shell` reads the cache and prints a table; it never runs an update command.

Network failures and uncertain installation ownership are reported as `unknown`, not as `up_to_date` and not as a guessed command.

## Build and install

Requirements: Go 1.22+ and a POSIX build environment.

Build locally without installing anything:

```sh
./install.sh --build
# or: make build
./dist/check-now list
```

Install under `~/.local` without changing shell profiles:

```sh
./install.sh --install
export PATH="$HOME/.local/bin:$PATH"
check-now doctor
check-now plugin create <id>
```

Use another prefix with `CHECK_NOW_PREFIX=/opt/check-now ./install.sh --install`. Shell integration is opt-in. When requested, the installer discovers an existing startup file and asks before changing it. If the detected file is wrong, answer `n` and enter the correct path. If no path is supplied, the binary remains installed and shell integration is skipped.

To select the startup file directly, including in noninteractive installs, use `--rc-file`:

```sh
./install.sh --install --shell zsh --shell-hook --rc-file "$HOME/.config/zsh/.zshrc"
```

Install and activate native completion alongside the update-check hook:

```sh
./install.sh --install --shell-hook --shell-completion
```

### Install a prebuilt release

Linux amd64 and arm64 releases are available without Go. The remote installer downloads the matching GitHub Release archive and verifies its SHA-256 checksum before replacing an existing installation. It needs a POSIX shell, `curl` or `wget`, `tar`, `sha256sum`, and common POSIX utilities. It does not require Go, Python, Cosign, or a package manager.

```sh
curl -fsSL https://check-now.pages.dev/install.sh | sh
```

Install or upgrade a pinned version and choose another prefix:

```sh
curl -fsSL https://check-now.pages.dev/install.sh | sh -s -- --version 0.3.2 --prefix "$HOME/.local"
```

Shell integration remains opt-in. The installer discovers a startup-file candidate for the selected shell and asks you to confirm it before editing. Zsh resolution uses Zsh startup configuration, including a custom `ZDOTDIR`; Bash, Fish, Ksh, and Dash use their shell-specific configuration paths and environment settings. The source installer also resolves PowerShell's `$PROFILE`. Answer `n` to enter another path. For unattended installs, pass `--rc-file PATH`; the supplied path is treated as your selection. If no path is confirmed, the binary is installed without hooks or completions, and the installer explains how to rerun setup.

For example, this installs the latest release, update hook, and Zsh completions, then confirms the discovered profile before editing it:

```sh
curl -fsSL https://check-now.pages.dev/install.sh | sh -s -- --version latest --prefix "$HOME/.local" --shell zsh --shell-hook --shell-completion
```

To avoid a prompt, provide the startup file explicitly with `--rc-file "$HOME/.config/zsh/.zshrc"`. After successful setup, the installer prints whether the hook and completion activation were added or already present, the completion script location, and a reminder to reload the profile or start a new shell. The installer supports hooks for Bash, Zsh, Fish, Ksh, and Dash, with completions for Bash, Zsh, and Fish. Add the chosen prefix's `bin` directory (by default, `~/.local/bin`) to `PATH` if it is not already present.

The `check-now version` command prints the installed release version. Source builds and the local `install.sh` workflow remain available.

### Release infrastructure

Pushing a `vX.Y.Z` tag that matches `VERSION` builds deterministic Linux amd64 and arm64 bundles, validates their contents and SHA-256 checksums, and publishes them to GitHub Releases. The Pages deployment workflow publishes `distribution/install.sh` at `https://check-now.pages.dev/install.sh` after a release is published and smoke-checks the public URL.

Configure the repository's `installer-production` environment with `CLOUDFLARE_API_TOKEN` (Pages Write) and `CLOUDFLARE_ACCOUNT_ID` before enabling release deployments. GitHub Actions creates the Pages project if needed, deploys the installer with the pinned Cloudflare Wrangler Action, and smoke-checks the public URL. No Cloudflare CLI setup is needed on a developer machine.

Override detection with `--shell bash`, `--shell zsh`, `--shell fish`, `--shell ksh`, `--shell dash`, or `--shell powershell`. Completion supports Bash, Zsh, Fish, and PowerShell (`pwsh`); Ksh and Dash remain supported only for the update-check hook.

## Shell integration

The hook only runs in interactive shells, remains silent when `check-now` is unavailable, and never evaluates plugin commands.

- Bash: `echo ". '$HOME/.local/share/check-now/check-now.sh'" >> ~/.bashrc`
- Zsh: `echo ". '$HOME/.local/share/check-now/check-now.sh'" >> ~/.zshrc`
- Ksh, mksh, or dash: `echo ". '$HOME/.local/share/check-now/check-now.sh'" >> ~/.profile`
- Fish: `echo "source '$HOME/.local/share/check-now/check-now.fish'" >> ~/.config/fish/config.fish`
- PowerShell: add `check-now shell | Out-Host` to `$PROFILE` when the executable is in `PATH`; the POSIX hook is not sourced by PowerShell.

The installers can add Bash/Zsh/Ksh/Dash/Fish hooks with `--shell-hook`; the source installer also adds `check-now shell | Out-Host` to the confirmed PowerShell `$PROFILE`. The source installer supports completion for Bash, Zsh, Fish, and PowerShell; the release installer supports Bash, Zsh, and Fish. Both write static scripts under `$PREFIX/share/check-now/completions/` and activate them with a separate idempotent profile marker. PowerShell completion resolves `$PROFILE` using `pwsh` or `powershell`.

## Commands

Run check-now without arguments, or use -h, --help, or help to see the command guide. Add --help to a command for its usage, options, and an example:

```sh
check-now help
check-now help plugin create
check-now refresh --help
```

The available commands are:

```sh
check-now version
check-now shell
check-now refresh
check-now refresh --force
check-now dismiss <plugin-id>
check-now list
check-now doctor
check-now plugin create <id>
check-now completion [bash|zsh|fish|powershell|pwsh]
```

version prints the installed release. shell displays cached update information and safe commands. refresh updates plugin information, and --force performs an immediate foreground refresh. dismiss hides displayed targets for a plugin ID. list shows discovered plugins. doctor reports per-setting, adapter, local probe, cache, and latest refresh findings. plugin create interactively scaffolds a local checker, with flags available for non-interactive use. completion prints a static script for a supported shell.

`doctor` stays local: it calls adapter `health` and `current`, reads target/current/control cache records and refresh history, and probes cache access with a temporary create/read/remove. It never refreshes remote sources, rewrites cache records, or applies repairs. Findings include status, a short explanation, and an optional suggested repair. A completed report exits successfully regardless of findings; invalid command usage still exits with an error. Configuration values and plugin stderr are not printed or stored in refresh history.

The human report groups configuration, adapter, and cache findings into sections. It uses terminal colors only when writing to an interactive terminal; redirected output stays plain. Missing refresh history is shown as `NOT CHECKED`, not as a warning.

Without a shell argument, `completion` detects the basename of `$SHELL`; when that is absent or unrecognized, `PSModulePath` selects PowerShell. Completion is static and side-effect free: it never discovers plugins, reads the cache, accesses the network, or dynamically enumerates custom plugin IDs.

`shell` is display-only. It probes each installed tool locally with `current`, reads cached targets, and schedules a bounded non-forced background `refresh` only when target data is missing or stale. Local probing is synchronous; remote discovery is asynchronous. Use `refresh --force` when you need an immediate foreground check, or `check-now dismiss <plugin-id>` to hide all currently displayed targets for a canonical plugin ID. A new target fingerprint or forced refresh clears that dismissal.

## Environment variables

All variables are optional. Invalid duration values fall back to their defaults during normal command execution and appear as findings in `check-now doctor`. Path-list variables use the platform path separator (`:` on Unix).

| Variable | Default | Purpose |
| --- | --- | --- |
| `CHECK_NOW_PLUGIN_DIRS` | empty | Additional plugin roots to search before built-ins. Duplicate IDs use the first discovered root. |
| `CHECK_NOW_BUILTIN_PLUGIN_DIR` | executable-relative locations | Replace built-in discovery with one directory. Useful for packaging and isolated tests. |
| `CHECK_NOW_STALE_INTERVAL` | `24h` | Cache age used when deciding whether cached targets need background refresh. |
| `CHECK_NOW_PLUGIN_TIMEOUT` | `5s` | Maximum time allowed for one plugin subprocess. Durations such as `2s` are accepted. |
| `XDG_CACHE_HOME` | `~/.cache` | Base directory for the `check-now` cache and refresh lock. |
| `CHECK_NOW_PLUGIN_ROOT` | `./plugins` | Destination root for `plugin create`; takes precedence over the first `CHECK_NOW_PLUGIN_DIRS` entry. |
| `CHECK_NOW_PREFIX` | `~/.local` | Installation prefix used by `install.sh --install`. Plugins are installed under `$CHECK_NOW_PREFIX/share/check-now/plugins`. |
| `NVM_DIR` | `~/.nvm` when set by the shell environment | Optional nvm root used by the Node plugin. nvm is used only when the running Node binary can be proven to belong to that root. |
| `CHECK_NOW_NODE_RELEASE_URL` | `https://nodejs.org/dist/index.json` | Node release index override. Intended for deterministic local fixtures or compatible mirrors. |
| `CHECK_NOW_BUN_RELEASE_URL` | Bun's official GitHub releases API | Bun release endpoint override for fixtures or compatible mirrors. |
| `CHECK_NOW_DENO_RELEASE_URL` | Deno's official GitHub releases API | Deno release endpoint override for fixtures or compatible mirrors. |
| `CHECK_NOW_FNM_RELEASE_URL` | fnm's official GitHub releases API | fnm release endpoint override for fixtures or compatible mirrors. |
| `CHECK_NOW_CODEX_RELEASE_URL` | Codex's official GitHub releases API | Codex release endpoint override for fixtures or compatible mirrors. |
| `CHECK_NOW_CLAUDE_RELEASE_URL` | Claude Code's official GitHub releases API | Claude Code release endpoint override for fixtures or compatible mirrors. |
| `CHECK_NOW_NODE_TARGET` | unset | Synthetic Node target for offline tests. Do not use as production detection input. |
| `CHECK_NOW_BUN_TARGET` | unset | Synthetic Bun target for offline tests. Do not use as production detection input. |
| `CHECK_NOW_DENO_TARGET` | unset | Synthetic Deno target for offline tests. Do not use as production detection input. |
| `CHECK_NOW_FNM_TARGET` | unset | Synthetic fnm target for offline tests. Do not use as production detection input. |
| `CHECK_NOW_CODEX_TARGET` | unset | Synthetic Codex target for offline tests. Do not use as production detection input. |
| `CHECK_NOW_CLAUDE_TARGET` | unset | Synthetic Claude Code target for offline tests. Do not use as production detection input. |

`install.sh` also reads the standard shell variables `SHELL`, `BASH_ENV`, and `ZDOTDIR` when detecting the profile to report or update. `HOME` is used by the shell and installer defaults. These are not check-now-specific configuration knobs.

Built-in plugins query their official release sources at refresh time. Node prefers a proven `fnm` or `nvm` installation and queries that manager's remote catalog. Claude Code recognizes its native version-store layout and global npm ownership, displaying `claude update` or the documented npm command only when the installation manager is proven. Codex recognizes its standalone release-store layout, global npm ownership, and Homebrew ownership; standalone updates use the official installer command. Network or parsing failures produce `unknown`; they are never treated as `up_to_date`.

When a checker can identify more than one useful target, `check-now shell` renders an aligned table with the tool name, current version, target version/track, safe update command, and source URL. For Node this can show both the newest release in the current major line and the newest release overall. A same-major target disappears as soon as the live current probe reaches it; a newer latest-major target remains informational. Shared name, current version, and source values are centered across grouped rows.

Plugins are trusted local executables. The host does not download, install, evaluate, or execute returned `argv`; it only displays safely quoted text. A failed, timed-out, malformed, or offline plugin leaves its last valid target cache untouched. Each attempted refresh records only its timestamp and a bounded outcome category so `doctor` can explain stale targets after failures.

See [plugin authoring](docs/plugin-authoring.md) for the protocol.

## Development and validation

The repository has no remote registry or network-dependent test suite. Run the complete local checks with:

```sh
make test       # offline plugin smoke test plus all Go tests
go test -race ./...
go vet ./...
make build      # produces dist/check-now and dist/plugins
```

For a generated plugin, run its `test.sh`, then `check-now list`, `check-now doctor`, and a host test before considering the implementation complete. See [plugin development](docs/plugin-development.md), [plugin protocol](docs/plugin-protocol.md), [architecture](docs/architecture.md), and [troubleshooting](docs/troubleshooting.md).

## Creating a plugin

The native generator is interactive and explains each field. ID, display name, and description are required; command/tool, official source URL, and installation manager are optional. Runtime defaults to `shell` and capabilities to `current,refresh,health`.

```sh
check-now plugin create nodejs --name "Node.js" --description "Checks Node.js installations" --tool node --source-url https://nodejs.org --runtime shell --manager fnm --capability current,refresh,health --non-interactive
```

Generated plugins include implementation guidance, an agent-facing `AGENTS.md`, and an offline smoke test. See [plugin development](docs/plugin-development.md), [plugin protocol](docs/plugin-protocol.md), and [architecture](docs/architecture.md).
