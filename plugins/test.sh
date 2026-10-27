#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
fake=$(mktemp -d)
trap 'rm -rf "$fake"' EXIT HUP INT TERM

cat >"$fake/curl" <<'EOF'
#!/bin/sh
url=
for arg in "$@"; do case "$arg" in http://*|https://*) url=$arg ;; esac; done
case "$url" in
  https://nodejs.org/dist/index.json) printf '%s\n' '[{"version":"v25.0.0"}]' ;;
  https://api.github.com/repos/oven-sh/bun/releases/latest) printf '%s\n' '{"tag_name":"bun-v2.0.0"}' ;;
  https://api.github.com/repos/denoland/deno/releases/latest) printf '%s\n' '{"tag_name":"v3.0.0"}' ;;
  https://api.github.com/repos/Schniz/fnm/releases/latest) printf '%s\n' '{"tag_name":"v2.0.0"}' ;;
  https://api.github.com/repos/openai/codex/releases/latest) printf '%s\n' '{"tag_name":"rust-v0.200.0"}' ;;
  https://api.github.com/repos/anthropics/claude-code/releases/latest) printf '%s\n' '{"tag_name":"v2.1.311"}' ;;
  *) exit 1 ;;
esac
EOF
chmod 700 "$fake/curl"
cat >"$fake/fake-tool" <<'EOF'
#!/bin/sh
case "$(basename "$0")" in
  node) printf '%s\n' 'v24.0.0' ;;
  bun) printf '%s\n' '1.0.0' ;;
  deno) printf '%s\n' '2.0.0' ;;
  fnm) case "${1:-}" in current) printf '%s\n' 'v24.0.0' ;; list-remote) printf '%s\nv24.20.0\nv25.0.0\n' ;; *) printf '%s\n' '1.0.0' ;; esac ;;
  codex) printf '%s\n' 'codex-cli 0.163.0' ;;
  claude) printf '%s\n' '2.1.310' ;;
esac
EOF
chmod 700 "$fake/fake-tool"
for id in node bun deno fnm codex claude; do ln -s fake-tool "$fake/$id"; done

for id in node bun deno fnm codex claude; do
  current=$(PATH="$fake:$PATH" "$root/$id/plugin.sh" <<EOF
{"protocol":1,"id":"$id","mode":"current"}
EOF
)
  case "$current" in *'"status":"up_to_date"'*) ;; *) echo "$id current probe failed: $current" >&2; exit 1 ;; esac
  output=$(PATH="$fake:$PATH" "$root/$id/plugin.sh" <<EOF
{"protocol":1,"id":"$id","mode":"refresh"}
EOF
)
  case "$id:$output" in node:*'"status":"update_available"'*) ;; bun:*'"status":"unknown"'*|deno:*'"status":"unknown"'*|fnm:*'"status":"unknown"'*|codex:*'"status":"unknown"'*|claude:*'"status":"unknown"'*) ;; *) echo "$id did not conservatively classify the fixture release: $output" >&2; exit 1 ;; esac
  case "$output" in *'"target_version"'*) ;; *) echo "$id omitted target_version: $output" >&2; exit 1 ;; esac
done

pretty=$(printf '%s\n' '{ "mode": "metadata", "id": "bun", "protocol": 1 }' | PATH="$fake:$PATH" "$root/bun/plugin.sh")
case "$pretty" in *'"name":"Bun"'*) ;; *) echo "pretty metadata request misclassified: $pretty" >&2; exit 1 ;; esac

offline=$(CHECK_NOW_BUN_RELEASE_URL=/not-found PATH="$fake:$PATH" "$root/bun/plugin.sh" <<EOF
{"protocol":1,"id":"bun","mode":"refresh"}
EOF
)
case "$offline" in *'"status":"unknown"'*) ;; *) echo "offline release lookup was not unknown: $offline" >&2; exit 1 ;; esac

unsafe=$(CHECK_NOW_BUN_TARGET='1.2"x' PATH="$fake:$PATH" "$root/bun/plugin.sh" <<EOF
{"protocol":1,"id":"bun","mode":"refresh"}
EOF
)
case "$unsafe" in *'1.2"x'*) echo "unsafe target escaped into protocol: $unsafe" >&2; exit 1 ;; esac
case "$unsafe" in *'"status":"unknown"'*) ;; *) echo "unsafe target was not rejected: $unsafe" >&2; exit 1 ;; esac

node_output=$(printf '%s\n' '{"protocol":1,"id":"node","mode":"refresh"}' | PATH="$fake:$PATH" "$root/node/plugin.sh")
printf '%s\n' "$node_output" | grep -F '"manager":"fnm"' | grep -F '"version":"24.20.0"' | grep -F '"version":"25.0.0"' | grep -F '"argv":["fnm","install","24.20.0"]' >/dev/null || { echo "bad fnm-backed node result: $node_output" >&2; exit 1; }

nvm_dir="$fake/nvm"
mkdir -p "$nvm_dir/versions/node/v24.0.0"
mkdir -p "$nvm_dir/versions/node/v24.0.0/bin"
mkdir -p "$fake/nvm-bin"
ln -s "$fake/node" "$nvm_dir/versions/node/v24.0.0/bin/node"
ln -s "$fake/curl" "$fake/nvm-bin/curl"
cat >"$nvm_dir/nvm.sh" <<'EOF'
nvm() { case "$1" in ls-remote) printf '%s\nv24.20.0\nv26.0.0\n' ;; *) : ;; esac; }
EOF
nvm_output=$(printf '%s\n' '{ "protocol": 1, "id": "node", "mode": "refresh" }' | PATH="$nvm_dir/versions/node/v24.0.0/bin:$fake/nvm-bin:$PATH" NVM_DIR="$nvm_dir" "$root/node/plugin.sh")
printf '%s\n' "$nvm_output" | grep -F '"manager":"nvm"' | grep -F '"version":"24.20.0"' | grep -F '"version":"26.0.0"' | grep -F '"argv":["nvm","install","24.20.0"]' >/dev/null || { echo "bad nvm-backed node result: $nvm_output" >&2; exit 1; }

echo "builtin plugin fixtures passed"
