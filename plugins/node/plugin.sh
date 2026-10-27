#!/bin/sh
set -u
# The Node checker owns detection, manager lookup, release selection, and protocol output.
id=node; name=Node; cmd=node; source=https://nodejs.org/en/download
request=$(cat)
now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
mode=$(printf '%s' "$request" | tr '\n' ' ' | sed -n 's/.*"mode"[[:space:]]*:[[:space:]]*"\([^" ]*\)".*/\1/p')
case "$mode" in
    metadata) printf '%s\n' '{"protocol":1,"id":"node","name":"Node","description":"Checks Node installations","tool":"node","source_url":"https://nodejs.org/en/download","runtime":"shell","capabilities":["current","refresh"]}'; exit 0 ;;
  current)
    if ! command -v "$cmd" >/dev/null 2>&1; then
      printf '{"protocol":1,"id":"%s","status":"not_installed","tool":"%s","checked_at":"%s"}\n' "$id" "$name" "$now"
    else
      v=$($cmd --version 2>/dev/null | head -n 1 | sed 's/[^0-9]*\([0-9][0-9.]*\).*/\1/' || true)
      if [ -n "$v" ]; then printf '{"protocol":1,"id":"%s","status":"up_to_date","tool":"%s","installation_kind":"runtime","current_version":"%s","checked_at":"%s"}\n' "$id" "$name" "$v" "$now"; else printf '{"protocol":1,"id":"%s","status":"unknown","tool":"%s","checked_at":"%s"}\n' "$id" "$name" "$now"; fi
    fi
    exit 0 ;;
  refresh) ;;
  *) echo 'invalid request mode' >&2; exit 2 ;;
esac
if ! command -v "$cmd" >/dev/null 2>&1; then
  printf '{"protocol":1,"id":"%s","status":"not_installed","tool":"%s","checked_at":"%s","explanation":"Command not found"}\n' "$id" "$name" "$now"
  exit 0
fi
version=$($cmd --version 2>/dev/null | head -n 1 | sed 's/[^0-9]*\([0-9][0-9.]*\).*/\1/' || true)
node_path=$(command -v "$cmd" 2>/dev/null || true)
release_url=${CHECK_NOW_NODE_RELEASE_URL:-https://nodejs.org/dist/index.json}
target_override=${CHECK_NOW_NODE_TARGET:-}
manager=; versions=
if command -v fnm >/dev/null 2>&1 && [ "$(fnm current 2>/dev/null | sed 's/[^0-9]*\([0-9][0-9.]*\).*/\1/' | head -n 1)" = "$version" ]; then
  manager=fnm
  versions=$(fnm list-remote 2>/dev/null | sed -n 's/^[[:space:]]*v\([0-9][0-9.]*\).*$/\1/p')
elif [ -n "${NVM_DIR:-}" ] && [ -r "$NVM_DIR/nvm.sh" ] && [ -d "$NVM_DIR/versions/node/v$version" ] && [ "${node_path#"$NVM_DIR"}" != "$node_path" ] && command -v bash >/dev/null 2>&1; then
  manager=nvm
  versions=$(bash --noprofile --norc -c '. "$1/nvm.sh"; nvm ls-remote --no-colors' check-now "$NVM_DIR" 2>/dev/null | sed -n 's/^[[:space:]]*v\([0-9][0-9.]*\).*$/\1/p')
elif [ -n "$target_override" ]; then
  versions=${target_override#v}
elif command -v curl >/dev/null 2>&1; then
  release=$(curl -fsSL --max-time 5 "$release_url" 2>/dev/null || true)
  versions=$(printf '%s' "$release" | tr '}' '\n' | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"v\{0,1\}\([0-9][0-9.]*\)".*/\1/p')
fi
select_targets() {
  awk -v major="${version%%.*}" 'function newer(a,b,aa,bb,i) { split(a,aa,"."); split(b,bb,"."); for (i=1; i<=3; i++) { if ((aa[i]+0) > (bb[i]+0)) return 1; if ((aa[i]+0) < (bb[i]+0)) return 0 } return 0 } { if ($0 !~ /^[0-9][0-9.]*$/) next; if (latest == "" || newer($0, latest)) latest=$0; split($0,p,"."); if ((p[1]+0) == (major+0) && (same == "" || newer($0, same))) same=$0 } END { print same "|" latest }'
}
selected=$(printf '%s\n' "$versions" | select_targets)
same_target=${selected%%|*}; latest_target=${selected#*|}
latest_target=${latest_target#v}; same_target=${same_target#v}
case "$latest_target" in ''|*[!0-9.]*|.*|*..*|*.) latest_target= ;; esac
case "$same_target" in ''|*[!0-9.]*|.*|*..*|*.) same_target= ;; esac
is_newer() { awk -v current="$1" -v target="$2" 'BEGIN { split(current,c,"."); split(target,t,"."); for (i=1; i<=3; i++) { if ((t[i]+0) > (c[i]+0)) exit 0; if ((t[i]+0) < (c[i]+0)) exit 1 } exit 1 }'; }
if [ -z "$version" ] || [ -z "$latest_target" ]; then
  printf '{"protocol":1,"id":"%s","status":"unknown","tool":"%s","checked_at":"%s","source_url":"%s","explanation":"Could not determine installed or latest release version"}\n' "$id" "$name" "$now" "$source"
  exit 0
fi
updates_json=
append_update() {
  scope=$1; track=$2; value=$3
  [ -n "$value" ] && is_newer "$version" "$value" || return 0
  entry='{"scope":"'"$scope"'","track":"'"$track"'","version":"'"$value"'"'
  if [ "$manager" = fnm ] || [ "$manager" = nvm ]; then entry="$entry,\"argv\":[\"$manager\",\"install\",\"$value\"]"; fi
  entry="$entry,\"source_url\":\"$source\"}"
  if [ -n "$updates_json" ]; then updates_json="$updates_json,$entry"; else updates_json=$entry; fi
}
if [ -n "$same_target" ] && [ "$same_target" != "$latest_target" ]; then append_update same_major "${version%%.*}.x" "$same_target"; fi
append_update latest latest "$latest_target"
if [ -n "$updates_json" ]; then
  status=unknown; explanation=',"explanation":"A newer official release exists, but the installation manager is not proven"'; manager_field=
  if [ "$manager" = fnm ] || [ "$manager" = nvm ]; then status=update_available; explanation=; manager_field=',"manager":"'"$manager"'"'; fi
  printf '{"protocol":1,"id":"%s","status":"%s","tool":"%s","installation_kind":"runtime"%s,"current_version":"%s","target_version":"%s","updates":[%s],"checked_at":"%s","source_url":"%s"%s}\n' "$id" "$status" "$name" "$manager_field" "$version" "$latest_target" "$updates_json" "$now" "$source" "$explanation"
else
  printf '{"protocol":1,"id":"%s","status":"up_to_date","tool":"%s","installation_kind":"runtime","current_version":"%s","checked_at":"%s","source_url":"%s"}\n' "$id" "$name" "$version" "$now" "$source"
fi
