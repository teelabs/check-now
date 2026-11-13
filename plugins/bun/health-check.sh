#!/bin/sh

# Emit stable, value-free health checks for a built-in adapter. The variable
# names are constants supplied by each trusted built-in plugin script.
check_now_health() {
  plugin_id=$1
  command_name=$2
  release_var=$3
  target_var=$4
  checked_at=$5
  eval "release_value=\${$release_var-}"
  eval "target_value=\${$target_var-}"

  if command -v "$command_name" >/dev/null 2>&1; then
    command_check='{"code":"command.available","status":"ok","summary":"The local command is available."}'
  else
    command_check='{"code":"command.available","status":"warning","summary":"The local command was not found on PATH.","repair":"Install the tool or correct PATH."}'
  fi

  if [ -z "$release_value" ]; then
    release_check='{"code":"config.release_url","status":"ok","summary":"The release endpoint uses the built-in default or a valid secure override."}'
  else
    case "$release_value" in
      https://?*)
        case "$release_value" in
          *[[:space:]]*) release_check='{"code":"config.release_url","status":"error","summary":"The release endpoint override is invalid.","repair":"Set the documented release URL override to an HTTPS endpoint."}' ;;
          *) release_check='{"code":"config.release_url","status":"ok","summary":"The release endpoint uses the built-in default or a valid secure override."}' ;;
        esac
        ;;
      *) release_check='{"code":"config.release_url","status":"error","summary":"The release endpoint override is invalid.","repair":"Set the documented release URL override to an HTTPS endpoint."}' ;;
    esac
  fi

  if [ -z "$target_value" ]; then
    target_check='{"code":"config.target","status":"ok","summary":"No synthetic target override is active."}'
  else
    case "$target_value" in
      *[!0-9.]*|.*|*..*|*.) target_check='{"code":"config.target","status":"error","summary":"The synthetic target override is invalid.","repair":"Use a dotted numeric version or clear the override."}' ;;
      *) target_check='{"code":"config.target","status":"warning","summary":"A synthetic target override is active and bypasses release discovery.","repair":"Clear the target override outside deterministic fixture testing."}' ;;
    esac
  fi

  printf '{"protocol":2,"id":"%s","checked_at":"%s","checks":[%s,%s,%s]}\n' \
    "$plugin_id" "$checked_at" "$command_check" "$release_check" "$target_check"
}
