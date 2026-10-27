#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
prefix=${CHECK_NOW_PREFIX:-"$HOME/.local"}
shell=auto
do_install=false
shell_hook=false

usage() {
  cat <<EOF
Usage: $0 [--build] [--install] [--shell NAME] [--shell-hook]

  --build       build dist/check-now and copy plugins (default)
  --install     install the executable and plugins under CHECK_NOW_PREFIX or ~/.local
  --shell NAME  bash, zsh, fish, ksh, dash, or powershell (default: detect)
  --shell-hook  add the hook to the detected shell profile after installation
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --build) do_install=false ;;
    --install) do_install=true ;;
    --shell) [ "$#" -ge 2 ] || { usage >&2; exit 2; }; shell=$2; shift ;;
    --shell-hook) shell_hook=true ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
  shift
done

if [ "$shell" = auto ]; then
  shell=${SHELL:-}
  if [ -z "$shell" ]; then
    shell=$(ps -p $$ -o comm= 2>/dev/null || true)
  fi
  shell=$(basename "$shell")
  case "$shell" in bash|zsh|fish|ksh|mksh|dash|powershell|pwsh) ;; *) shell=unknown ;; esac
fi
printf 'Detected shell: %s\n' "$shell"

mkdir -p "$root/dist"
(cd "$root" && go build -o dist/check-now ./cmd/check-now)
rm -rf "$root/dist/plugins"
cp -R "$root/plugins" "$root/dist/plugins"
printf 'Built: %s\n' "$root/dist/check-now"

if [ "$do_install" = true ]; then
  mkdir -p "$prefix/bin" "$prefix/share/check-now"
  cp "$root/dist/check-now" "$prefix/bin/check-now"
  rm -rf "$prefix/share/check-now/plugins"
  cp -R "$root/dist/plugins" "$prefix/share/check-now/plugins"
	cp "$root/check-now.sh" "$root/check-now.fish" "$prefix/share/check-now/"
  printf 'Installed: %s/bin/check-now\n' "$prefix"
  printf 'Plugins: %s/share/check-now/plugins\n' "$prefix"
fi

profile=
case "$shell" in
  bash) profile=${BASH_ENV:-"$HOME/.bashrc"} ;;
  zsh) profile=${ZDOTDIR:-"$HOME"}/.zshrc ;;
  ksh|mksh|dash) profile=$HOME/.profile ;;
  fish) profile=$HOME/.config/fish/config.fish ;;
  powershell) profile='PowerShell profile' ;;
esac

if [ "$shell_hook" = true ]; then
  [ "$do_install" = true ] || { echo '--shell-hook requires --install' >&2; exit 2; }
  case "$shell" in
    bash|zsh|ksh|mksh|dash)
      mkdir -p "$(dirname "$profile")"
      marker='# check-now shell integration'
      if ! grep -Fqx "$marker" "$profile" 2>/dev/null; then
        { printf '\n%s\n' "$marker"; printf '%s\n' ". '$prefix/share/check-now/check-now.sh'"; } >> "$profile"
      fi
      ;;
    fish)
      mkdir -p "$(dirname "$profile")"
      marker='# check-now shell integration'
      if ! grep -Fqx "$marker" "$profile" 2>/dev/null; then
        { printf '\n%s\n' "$marker"; printf '%s\n' "source '$prefix/share/check-now/check-now.fish'"; } >> "$profile"
      fi
      ;;
    powershell) echo 'PowerShell hook is printed in README; profile was not modified.'; exit 0 ;;
    *) echo 'Cannot configure an unknown shell.' >&2; exit 2 ;;
  esac
  printf 'Shell hook configured in %s\n' "$profile"
else
  printf 'Shell hook not changed. See README.md for the command for %s.\n' "$shell"
fi
