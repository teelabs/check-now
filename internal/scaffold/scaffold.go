package scaffold

import (
	"check-now/internal/protocol"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Options struct {
	Root, ID, Name, Description, Tool, SourceURL, Runtime, Manager string
	Capabilities                                                   []string
	Force                                                          bool
}

var validID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func Create(root string, o Options) (string, error) {
	if !validID.MatchString(o.ID) {
		return "", fmt.Errorf("invalid plugin ID %q: use lowercase letters, numbers, '.', '_' or '-'", o.ID)
	}
	if strings.TrimSpace(o.Name) == "" || strings.TrimSpace(o.Description) == "" {
		return "", fmt.Errorf("name and description are required")
	}
	if o.Runtime == "" {
		o.Runtime = "shell"
	}
	if len(o.Capabilities) == 0 {
		o.Capabilities = []string{"refresh"}
	}
	dir := filepath.Join(root, o.ID)
	if _, err := os.Stat(dir); err == nil {
		if !o.Force {
			return "", fmt.Errorf("plugin directory already exists: %s (use --force to replace it)", dir)
		}
		if err := os.RemoveAll(dir); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	m := protocol.Metadata{Protocol: protocol.Version, ID: o.ID, Name: o.Name, Description: o.Description, Tool: o.Tool, SourceURL: o.SourceURL, Runtime: o.Runtime, Manager: o.Manager, Capabilities: o.Capabilities}
	meta, _ := json.Marshal(m)
	refreshPrefix := `{"protocol":1,"id":` + jsonString(o.ID) + `,"status":"unknown","tool":` + jsonString(o.Name) + `,"checked_at":"`
	refreshSuffix := `"}`
	plugin := "#!/bin/sh\nset -u\n# Generated check-now plugin. stdout must contain exactly one JSON response.\nrequest=$(cat)\nnow=$(date -u +%Y-%m-%dT%H:%M:%SZ)\n# The host sends one request; metadata describes the plugin and refresh reports its state.\n# Diagnostics must go to stderr, and this starter never executes update commands.\n# TODO: detect installation.\n# TODO: look up the installed and target versions.\n# TODO: compare versions using the tool's release policy.\n# TODO: identify the manager only when it is certain.\n# TODO: add safe update argv only when manager authority is known.\nmode=$(printf '%s' \"$request\" | tr '\\n' ' ' | sed -n 's/.*\"mode\"[[:space:]]*:[[:space:]]*\"\\([^\" ]*\\)\".*/\\1/p')\ncase \"$mode\" in\n  metadata) printf '%s\\n' " + shellQuote(string(meta)) + " ;;\n  refresh) printf '%s\\n' " + shellQuote(refreshPrefix) + "\"$now\"" + shellQuote(refreshSuffix) + " ;;\n  *) echo 'invalid request mode' >&2; exit 2 ;;\nesac\n"
	readme := fmt.Sprintf("# %s\n\nGenerated plugin ID: `%s`.\n\n%s\n\n## Selected metadata\n\n- Runtime: %s\n- Tool/command: %s\n- Official source URL: %s\n- Installation manager: %s\n- Capabilities: %s\n\nOptional blank fields are omitted from metadata. stdout is reserved for one JSON response and diagnostics belong on stderr.\n\n## Implementation workflow\n\nRead `AGENTS.md`, `plugin.sh`, and `docs/plugin-protocol.md`; add a deterministic fixture to `test.sh` first; implement detection, version lookup, comparison, manager identification, and safe argv only when authority is clear. Unknown is the conservative result.\n\nRun `./test.sh`, then `check-now list` and `check-now doctor`.\n\nResults use `not_installed`, `up_to_date`, `update_available`, or `unknown`. `argv` is display-only structured data; the host never executes it.\n", o.Name, o.ID, o.Description, o.Runtime, blank(o.Tool), blank(o.SourceURL), blank(o.Manager), strings.Join(o.Capabilities, ", "))
	agents := "# AI-assisted implementation guidance\n\nRead this file, `README.md`, `plugin.sh`, and `docs/plugin-protocol.md` before editing.\n\n- Preserve the fixed metadata and refresh result schemas and the plugin boundary.\n- Make changes test-first through `test.sh`; keep stdout protocol-pure and logs on stderr.\n- Return conservative `unknown` when installation or update authority is unclear.\n- Never execute plugin-provided update commands during implementation or tests.\n- Use local fixtures or fake commands, never live network APIs.\n- Validate with `./test.sh`, `check-now list`, and `check-now doctor`.\n- Document new assumptions, managers, source URLs, and update commands.\n- Do not modify unrelated plugins or host behavior without corresponding tests.\n"
	test := fmt.Sprintf("#!/bin/sh\nset -eu\nroot=$(CDPATH= cd -- \"$(dirname -- \"$0\")\" && pwd)\nmeta=$(printf '{\"protocol\":1,\"id\":\"%s\",\"mode\":\"metadata\"}\\n' | \"$root/plugin.sh\" 2>\"$root/.stderr\")\nprintf '%%s\\n' \"$meta\" | awk 'NR != 1 { exit 1 }'\ncase \"$meta\" in *'\"protocol\":1'*'\"id\":\"%s\"'*'\"name\":\"%s\"'*) ;; *) echo 'invalid metadata' >&2; exit 1 ;; esac\nrefresh=$(printf '{\"protocol\":1,\"id\":\"%s\",\"mode\":\"refresh\"}\\n' | \"$root/plugin.sh\" 2>>\"$root/.stderr\")\nprintf '%%s\\n' \"$refresh\" | awk 'NR != 1 { exit 1 }'\ncase \"$refresh\" in *'\"protocol\":1'*'\"status\":\"unknown\"'*) ;; *) echo 'invalid refresh' >&2; exit 1 ;; esac\n[ ! -s \"$root/.stderr\" ] || { echo 'unexpected diagnostics' >&2; exit 1; }\nrm -f \"$root/.stderr\"\n", o.ID, o.ID, o.Name, o.ID)
	files := []struct {
		name, body string
		mode       os.FileMode
	}{{"plugin.sh", plugin, 0755}, {"README.md", readme, 0644}, {"AGENTS.md", agents, 0644}, {"test.sh", test, 0755}}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.body), f.mode); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func blank(s string) string {
	if s == "" {
		return "(omitted)"
	}
	return s
}

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
