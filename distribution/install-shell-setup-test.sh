#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
temp=$(mktemp -d "${TMPDIR:-/tmp}/check-now-shell-setup-test.XXXXXX")
trap 'rm -rf "$temp"' EXIT HUP INT TERM
mkdir -p "$temp/mock-bin" "$temp/payload/plugins" "$temp/home" "$temp/prefix"

cat >"$temp/payload/check-now" <<'MOCK'
#!/bin/sh
if [ "$1" = completion ]; then
  printf '# generated %s completion\n' "$2"
fi
MOCK
printf 'plugin fixture\n' >"$temp/payload/plugins/example"
printf 'hook fixture\n' >"$temp/payload/check-now.sh"
printf 'fish hook fixture\n' >"$temp/payload/check-now.fish"
printf '1.2.3\n' >"$temp/payload/VERSION"
chmod +x "$temp/payload/check-now"
tar -czf "$temp/check-now-linux-amd64.tar.gz" -C "$temp/payload" .
(cd "$temp" && sha256sum check-now-linux-amd64.tar.gz >check-now-linux-amd64.tar.gz.sha256)

cat >"$temp/mock-bin/curl" <<'MOCK'
#!/bin/sh
set -eu
out=
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then out=$2; shift 2; else shift; fi
done
if [ -z "$out" ]; then
  cat "$CHECK_NOW_INSTALLER_SCRIPT"
  exit 0
fi
case "$out" in
  */check-now-linux-amd64.tar.gz) cp "$CHECK_NOW_FIXTURE/check-now-linux-amd64.tar.gz" "$out" ;;
  */check-now-linux-amd64.tar.gz.sha256) cp "$CHECK_NOW_FIXTURE/check-now-linux-amd64.tar.gz.sha256" "$out" ;;
  *) exit 2 ;;
esac
MOCK
cat >"$temp/mock-bin/uname" <<'MOCK'
#!/bin/sh
case "$1" in -m) printf 'x86_64\n' ;; -s) printf 'Linux\n' ;; *) exit 2 ;; esac
MOCK
cat >"$temp/mock-bin/zsh" <<'MOCK'
#!/bin/sh
printf 'zsh startup noise\nCHECK_NOW_PROFILE=%s/.zshrc\n' "$CHECK_NOW_TEST_ZDOTDIR"
MOCK
cat >"$temp/mock-bin/pwsh" <<'MOCK'
#!/bin/sh
printf 'CHECK_NOW_PROFILE=%s\n' "$CHECK_NOW_TEST_POWERSHELL_PROFILE"
MOCK
chmod +x "$temp/mock-bin/curl" "$temp/mock-bin/uname" "$temp/mock-bin/zsh" "$temp/mock-bin/pwsh"

export CHECK_NOW_FIXTURE="$temp"
export CHECK_NOW_INSTALLER_SCRIPT="$repo/distribution/install.sh"
export CHECK_NOW_PREFIX="$temp/prefix"
export CHECK_NOW_TEST_ZDOTDIR="$temp/custom-zsh"
export HOME="$temp/home"
export PATH="$temp/mock-bin:$PATH"
mkdir -p "$CHECK_NOW_TEST_ZDOTDIR" "$temp/config/fish"
touch "$CHECK_NOW_TEST_ZDOTDIR/.zshrc" "$temp/config/fish/config.fish"

# A plain install clearly reports that profile setup was not requested.
output=$(setsid sh "$repo/distribution/install.sh" --version 1.2.3 --prefix "$temp/no-request-prefix" </dev/null 2>&1)
printf '%s\n' "$output" | grep -Fq 'Shell hook and completions were not requested' || {
  printf 'plain install did not report that shell setup was not requested\n' >&2
  exit 1
}

run_tty() {
  input=$1
  shift
  printf '%b' "$input" | script -qefc "$*" /dev/null
}

# A discovered custom Zsh rc file must be shown and confirmed before it changes.
output=$(run_tty 'y\n' "curl -fsSL https://check-now.pages.dev/install.sh | sh -s -- --version 1.2.3 --prefix '$CHECK_NOW_PREFIX' --shell zsh --shell-hook --shell-completion")
printf '%s\n' "$output" | grep -Fq "$CHECK_NOW_TEST_ZDOTDIR/.zshrc" || {
  printf 'Zsh custom rc file was not reported\n' >&2
  exit 1
}
grep -Fq '# check-now shell integration' "$CHECK_NOW_TEST_ZDOTDIR/.zshrc" || {
  printf 'Zsh hook was not installed in the confirmed rc file\n' >&2
  exit 1
}
grep -Fq '# check-now shell completion' "$CHECK_NOW_TEST_ZDOTDIR/.zshrc" || {
  printf 'Zsh completion activation was not installed in the confirmed rc file\n' >&2
  exit 1
}
grep -Fq 'start a new shell or reload' "$CHECK_NOW_TEST_ZDOTDIR/.zshrc" && {
  printf 'reload guidance should be printed, not added to the rc file\n' >&2
  exit 1
}
printf '%s\n' "$output" | grep -Fq 'Start a new shell or reload' || {
  printf 'reload guidance was not printed\n' >&2
  exit 1
}
printf '%s\n' "$output" | grep -Fq "$CHECK_NOW_PREFIX/share/check-now/check-now.sh" || {
  printf 'hook script path was not reported\n' >&2
  exit 1
}
printf '%s\n' "$output" | grep -Fq "$CHECK_NOW_PREFIX/share/check-now/completions/_check-now" || {
  printf 'completion script path was not reported\n' >&2
  exit 1
}

# A rejected discovered path prompts for a replacement; blank skips integration.
rm -rf "$CHECK_NOW_PREFIX"
mkdir -p "$CHECK_NOW_PREFIX"
rm -f "$CHECK_NOW_TEST_ZDOTDIR/.zshrc"
touch "$CHECK_NOW_TEST_ZDOTDIR/.zshrc"
output=$(run_tty 'n\n\n' "sh '$repo/distribution/install.sh' --version 1.2.3 --prefix '$CHECK_NOW_PREFIX' --shell zsh --shell-hook --shell-completion")
[ -x "$CHECK_NOW_PREFIX/bin/check-now" ] || {
  printf 'binary installation did not finish after integration was skipped\n' >&2
  exit 1
}
if grep -Fq '# check-now shell integration' "$CHECK_NOW_TEST_ZDOTDIR/.zshrc"; then
  printf 'declined rc file was modified\n' >&2
  exit 1
fi
printf '%s\n' "$output" | grep -Fq 'shell hook and completions were not installed' || {
  printf 'skipped integration message was not printed\n' >&2
  exit 1
}
printf '%s\n' "$output" | grep -Fq 'Run the installer again' || {
  printf 'rerun guidance was not printed\n' >&2
  exit 1
}

# Rejecting a discovered file allows the user to supply the correct custom path.
replacement_rc="$temp/replacement/zshrc"
output=$(run_tty "n\n$replacement_rc\n" "sh '$repo/distribution/install.sh' --version 1.2.3 --prefix '$CHECK_NOW_PREFIX' --shell zsh --shell-hook")
grep -Fq '# check-now shell integration' "$replacement_rc" || {
  printf 'replacement rc path was not used after rejecting discovery\n' >&2
  exit 1
}
if grep -Fq '# check-now shell integration' "$CHECK_NOW_TEST_ZDOTDIR/.zshrc"; then
  printf 'rejected discovered rc file was modified\n' >&2
  exit 1
fi

# Shell-specific discovery works for a non-Zsh shell and honors XDG_CONFIG_HOME.
touch "$HOME/.bashrc"
output=$(run_tty 'y\n' "env CHECK_NOW_PREFIX='$temp/bash-prefix' sh '$repo/distribution/install.sh' --version 1.2.3 --prefix '$temp/bash-prefix' --shell bash --shell-hook")
printf '%s\n' "$output" | grep -Fq "$HOME/.bashrc" || {
  printf 'Bash rc file was not discovered and reported\n' >&2
  exit 1
}
touch "$HOME/.profile"
output=$(run_tty 'y\n' "env CHECK_NOW_PREFIX='$temp/dash-prefix' sh '$repo/distribution/install.sh' --version 1.2.3 --prefix '$temp/dash-prefix' --shell dash --shell-hook")
printf '%s\n' "$output" | grep -Fq "$HOME/.profile" || {
  printf 'Dash profile was not discovered and reported\n' >&2
  exit 1
}
output=$(run_tty 'y\n' "env CHECK_NOW_PREFIX='$temp/fish-prefix' XDG_CONFIG_HOME='$temp/config' sh '$repo/distribution/install.sh' --version 1.2.3 --prefix '$temp/fish-prefix' --shell fish --shell-hook")
printf '%s\n' "$output" | grep -Fq "$temp/config/fish/config.fish" || {
  printf 'Fish rc file from XDG_CONFIG_HOME was not discovered\n' >&2
  exit 1
}

# Without a terminal or explicit path, binaries install but profile files stay untouched.
: >"$temp/config/fish/config.fish"
output=$(env CHECK_NOW_PREFIX="$temp/noninteractive-prefix" XDG_CONFIG_HOME="$temp/config" setsid sh "$repo/distribution/install.sh" --version 1.2.3 --prefix "$temp/noninteractive-prefix" --shell fish --shell-hook </dev/null 2>&1)
[ -x "$temp/noninteractive-prefix/bin/check-now" ] || {
  printf 'noninteractive setup did not preserve the binary installation\n' >&2
  exit 1
}
if grep -Fq '# check-now shell integration' "$temp/config/fish/config.fish"; then
  printf 'noninteractive setup modified a profile without confirmation\n' >&2
  exit 1
fi
printf '%s\n' "$output" | grep -Fq 'No shell profile was confirmed' || {
  printf 'noninteractive skip reason was not reported\n' >&2
  exit 1
}

# Explicit --rc-file is accepted without a terminal and repeated setup is idempotent.
explicit_prefix="$temp/prefix with spaces"
explicit_rc="$temp/explicit path/zshrc"
output=$(setsid sh "$repo/distribution/install.sh" --version 1.2.3 --prefix "$explicit_prefix" --shell zsh --shell-hook --shell-completion --rc-file "$explicit_rc" </dev/null 2>&1)
printf '%s\n' "$output" | grep -Fq 'Shell hook added' || {
  printf 'explicit rc file setup was not reported as added\n' >&2
  exit 1
}

# An existing non-file path is rejected instead of blocking while the installer reads it.
nonfile_rc="$temp/nonfile-zshrc"
mkfifo "$nonfile_rc"
nonfile_output="$temp/nonfile-output"
setsid sh "$repo/distribution/install.sh" --version 1.2.3 --prefix "$temp/nonfile-prefix" --shell zsh --shell-hook --rc-file "$nonfile_rc" </dev/null >"$nonfile_output" 2>&1 &
nonfile_pid=$!
nonfile_wait=0
while kill -0 "$nonfile_pid" 2>/dev/null && [ "$nonfile_wait" -lt 50 ]; do
  sleep 0.1
  nonfile_wait=$((nonfile_wait + 1))
done
if kill -0 "$nonfile_pid" 2>/dev/null; then
  /bin/kill -TERM -- "-$nonfile_pid" 2>/dev/null || true
  sleep 0.1
  /bin/kill -KILL -- "-$nonfile_pid" 2>/dev/null || true
  printf 'non-file rc path caused the installer to block\n' >&2
  exit 1
fi
wait "$nonfile_pid"
output=$(cat "$nonfile_output")
printf '%s\n' "$output" | grep -Fq 'Profile path must name a regular file or a path that does not exist yet' || {
  printf 'non-file rc path was not rejected clearly\n' >&2
  exit 1
}

# A dangling symlink is rejected so the installer cannot create its unknown target.
symlink_target="$temp/dangling-target"
symlink_rc="$temp/dangling-zshrc"
ln -s "$symlink_target" "$symlink_rc"
output=$(setsid sh "$repo/distribution/install.sh" --version 1.2.3 --prefix "$temp/dangling-prefix" --shell zsh --shell-hook --rc-file "$symlink_rc" </dev/null 2>&1)
printf '%s\n' "$output" | grep -Fq 'Profile path must name a regular file or a path that does not exist yet' || {
  printf 'distribution installer did not reject a dangling profile symlink\n' >&2
  exit 1
}
[ ! -e "$symlink_target" ] || {
  printf 'distribution installer created a dangling symlink target\n' >&2
  exit 1
}

output=$(setsid sh "$repo/distribution/install.sh" --version 1.2.3 --prefix "$explicit_prefix" --shell zsh --shell-hook --shell-completion --rc-file "$explicit_rc" </dev/null 2>&1)
printf '%s\n' "$output" | grep -Fq 'Shell hook already present' || {
  printf 'repeat setup was not reported as already present\n' >&2
  exit 1
}
[ "$(grep -Fc '# check-now shell integration' "$explicit_rc")" -eq 1 ] || {
  printf 'repeat setup duplicated the hook marker\n' >&2
  exit 1
}
[ "$(grep -Fc '# check-now shell completion' "$explicit_rc")" -eq 1 ] || {
  printf 'repeat setup duplicated the completion marker\n' >&2
  exit 1
}

# A pre-existing marker without a trailing newline still receives a separate activation line.
truncated_rc="$temp/truncated/zshrc"
mkdir -p "$(dirname "$truncated_rc")"
printf '# check-now shell integration' >"$truncated_rc"
setsid sh "$repo/distribution/install.sh" --version 1.2.3 --prefix "$CHECK_NOW_PREFIX" --shell zsh --shell-hook --rc-file "$truncated_rc" </dev/null >/dev/null 2>&1
grep -Fqx ". '$CHECK_NOW_PREFIX/share/check-now/check-now.sh'" "$truncated_rc" || {
  printf 'activation after a marker without a trailing newline was malformed\n' >&2
  exit 1
}

# The repository-local installer accepts the same explicit profile option.
local_rc="$temp/local-bashrc"
unsupported_rc="$temp/unsupported-rc"
if CHECK_NOW_PREFIX="$temp/unsupported-prefix" HOME="$HOME" sh "$repo/install.sh" --install --shell not-a-shell --shell-hook --rc-file "$unsupported_rc" >"$temp/unsupported-output" 2>&1; then
  printf 'local installer accepted an unsupported shell for integration\n' >&2
  exit 1
fi
if [ -e "$unsupported_rc" ]; then
  printf 'local installer modified a profile for an unsupported shell\n' >&2
  exit 1
fi
grep -Fq 'Cannot configure shell' "$temp/unsupported-output" || {
  printf 'local installer did not explain the unsupported shell\n' >&2
  exit 1
}
output=$(CHECK_NOW_PREFIX="$temp/local-prefix" HOME="$HOME" sh "$repo/install.sh" --install --shell bash --shell-hook --shell-completion --rc-file "$local_rc" 2>&1)
printf '%s\n' "$output" | grep -Fq 'Shell hook added to' || {
  printf 'local installer did not report its hook setup\n' >&2
  exit 1
}
printf '%s\n' "$output" | grep -Fq "$temp/local-prefix/share/check-now/completions/check-now.bash" || {
  printf 'local installer did not report its completion script\n' >&2
  exit 1
}
grep -Fq '# check-now shell integration' "$local_rc" || {
  printf 'local installer did not modify the explicitly selected profile\n' >&2
  exit 1
}
output=$(CHECK_NOW_PREFIX="$temp/local-prefix" HOME="$HOME" sh "$repo/install.sh" --install --shell bash --shell-hook --shell-completion --rc-file "$local_rc" 2>&1)
printf '%s\n' "$output" | grep -Fq 'Shell hook already present in' || {
  printf 'local installer did not report existing hook setup\n' >&2
  exit 1
}
[ "$(grep -Fc '# check-now shell integration' "$local_rc")" -eq 1 ] || {
  printf 'local installer duplicated its hook marker\n' >&2
  exit 1
}

local_symlink_target="$temp/local-dangling-target"
local_symlink_rc="$temp/local-dangling-bashrc"
ln -s "$local_symlink_target" "$local_symlink_rc"
output=$(CHECK_NOW_PREFIX="$temp/local-prefix" HOME="$HOME" sh "$repo/install.sh" --install --shell bash --shell-hook --rc-file "$local_symlink_rc" 2>&1)
printf '%s\n' "$output" | grep -Fq 'Profile path must name a regular file or a path that does not exist yet' || {
  printf 'local installer did not reject a dangling profile symlink clearly\n' >&2
  exit 1
}
[ ! -e "$local_symlink_target" ] || {
  printf 'local installer created a dangling symlink target\n' >&2
  exit 1
}

# PowerShell profile discovery is confirmed before the local hook is added.
CHECK_NOW_TEST_POWERSHELL_PROFILE="$temp/powershell/Microsoft.PowerShell_profile.ps1"
export CHECK_NOW_TEST_POWERSHELL_PROFILE
mkdir -p "$(dirname "$CHECK_NOW_TEST_POWERSHELL_PROFILE")"
touch "$CHECK_NOW_TEST_POWERSHELL_PROFILE"
output=$(run_tty 'y\n' "env CHECK_NOW_PREFIX='$temp/local-pwsh-prefix' HOME='$HOME' sh '$repo/install.sh' --install --shell powershell --shell-hook")
printf '%s\n' "$output" | grep -Fq "$CHECK_NOW_TEST_POWERSHELL_PROFILE" || {
  printf 'PowerShell profile was not discovered and confirmed\n' >&2
  exit 1
}
grep -Fqx 'check-now shell | Out-Host' "$CHECK_NOW_TEST_POWERSHELL_PROFILE" || {
  printf 'PowerShell update hook was not installed in the confirmed profile\n' >&2
  exit 1
}
printf 'installer shell setup discovery and confirmation passed\n'
