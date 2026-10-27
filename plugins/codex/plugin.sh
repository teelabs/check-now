#!/bin/sh
set -u
# This file is the complete Codex CLI plugin. The host invokes it directly.
id=codex; name="Codex CLI"; cmd=codex; source=https://github.com/openai/codex
request=$(cat); now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
mode=$(printf '%s' "$request" | tr '\n' ' ' | sed -n 's/.*"mode"[[:space:]]*:[[:space:]]*"\([^" ]*\)".*/\1/p')
case "$mode" in metadata) printf '%s\n' '{"protocol":1,"id":"codex","name":"Codex CLI","description":"Checks Codex CLI installations","tool":"codex","source_url":"https://github.com/openai/codex","runtime":"shell","capabilities":["current","refresh"]}'; exit 0;; current) if ! command -v "$cmd" >/dev/null 2>&1; then printf '{"protocol":1,"id":"%s","status":"not_installed","tool":"%s","checked_at":"%s"}\n' "$id" "$name" "$now"; else v=$($cmd --version 2>/dev/null | head -n 1 | sed 's/[^0-9]*\([0-9][0-9.]*\).*/\1/' || true); if [ -n "$v" ]; then printf '{"protocol":1,"id":"%s","status":"up_to_date","tool":"%s","installation_kind":"cli","current_version":"%s","checked_at":"%s"}\n' "$id" "$name" "$v" "$now"; else printf '{"protocol":1,"id":"%s","status":"unknown","tool":"%s","checked_at":"%s"}\n' "$id" "$name" "$now"; fi; fi; exit 0;; refresh) ;; *) echo 'invalid request mode' >&2; exit 2;; esac
if ! command -v "$cmd" >/dev/null 2>&1; then printf '{"protocol":1,"id":"%s","status":"not_installed","tool":"%s","checked_at":"%s","explanation":"Command not found"}\n' "$id" "$name" "$now"; exit 0; fi
version=$($cmd --version 2>/dev/null | head -n 1 | sed 's/[^0-9]*\([0-9][0-9.]*\).*/\1/' || true)
release_url=${CHECK_NOW_CODEX_RELEASE_URL:-https://api.github.com/repos/openai/codex/releases/latest}
target=${CHECK_NOW_CODEX_TARGET:-}
if [ -z "$target" ] && command -v curl >/dev/null 2>&1; then release=$(curl -fsSL --max-time 5 "$release_url" 2>/dev/null || true); target=$(printf '%s' "$release" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"rust-v\([0-9][0-9.]*\)".*/\1/p' | head -n 1); fi
target=${target#v}
case "$target" in ''|*[!0-9.]*|.*|*..*|*.) target= ;; esac
is_newer() { awk -v current="$1" -v target="$2" 'BEGIN { split(current,c,"."); split(target,t,"."); for (i=1; i<=3; i++) { if ((t[i]+0) > (c[i]+0)) exit 0; if ((t[i]+0) < (c[i]+0)) exit 1 } exit 1 }'; }
if [ -z "$version" ] || [ -z "$target" ]; then printf '{"protocol":1,"id":"%s","status":"unknown","tool":"%s","checked_at":"%s","source_url":"%s","explanation":"Could not determine installed or latest release version"}\n' "$id" "$name" "$now" "$source"
elif is_newer "$version" "$target"; then printf '{"protocol":1,"id":"%s","status":"unknown","tool":"%s","installation_kind":"cli","current_version":"%s","target_version":"%s","checked_at":"%s","source_url":"%s","explanation":"A newer official release exists, but the installation manager is not proven"}\n' "$id" "$name" "$version" "$target" "$now" "$source"
else printf '{"protocol":1,"id":"%s","status":"up_to_date","tool":"%s","installation_kind":"cli","current_version":"%s","checked_at":"%s","source_url":"%s"}\n' "$id" "$name" "$version" "$now" "$source"; fi
