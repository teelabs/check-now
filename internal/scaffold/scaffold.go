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
		o.Capabilities = []string{"current", "refresh"}
	}
	if !has(o.Capabilities, "current") {
		o.Capabilities = append([]string{"current"}, o.Capabilities...)
	}
	if !has(o.Capabilities, "refresh") {
		o.Capabilities = append(o.Capabilities, "refresh")
	}
	dir := filepath.Join(root, o.ID)
	if _, err := os.Stat(dir); err == nil {
		if !o.Force {
			return "", fmt.Errorf("plugin directory already exists: %s (use --force to replace it)", dir)
		}
		if err = os.RemoveAll(dir); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	m := protocol.Metadata{Protocol: 1, ID: o.ID, Name: o.Name, Description: o.Description, Tool: o.Tool, SourceURL: o.SourceURL, Runtime: o.Runtime, Manager: o.Manager, Capabilities: o.Capabilities}
	mb, _ := json.Marshal(m)
	unknown := `{"protocol":1,"id":` + quote(o.ID) + `,"status":"unknown","tool":` + quote(o.Name) + `,"explanation":"TODO: implement detection and target lookup","checked_at":"`
	suffix := `"}`
	plugin := "#!/bin/sh\nset -u\nrequest=$(cat)\nnow=$(date -u +%Y-%m-%dT%H:%M:%SZ)\nmode=$(printf '%s' \"$request\" | tr '\\n' ' ' | sed -n 's/.*\"mode\"[[:space:]]*:[[:space:]]*\"\\([^\" ]*\\)\".*/\\1/p')\ncase \"$mode\" in\n  metadata) printf '%s\\n' " + shellQuote(string(mb)) + " ;;\n  current) printf '%s\\n' " + shellQuote(unknown) + "\"$now\"" + shellQuote(suffix) + " ;;\n  refresh) printf '%s\\n' " + shellQuote(unknown) + "\"$now\"" + shellQuote(suffix) + " ;;\n  *) echo 'invalid request mode' >&2; exit 2 ;;\nesac\n"
	readme := fmt.Sprintf("# %s\n\nGenerated plugin ID: `%s`.\n\n%s\n\nCapabilities: %s\n\nTODO: implement installation detection, target lookup, comparison, and manager-safe argv. Optional fields may be blank. stdout is reserved for one JSON response; diagnostics belong on stderr.\n", o.Name, o.ID, o.Description, strings.Join(o.Capabilities, ", "))
	agents := "# AI-assisted implementation guidance\n\nImplement current as a local-only probe and refresh as target discovery. Keep stdout protocol-pure, use offline fixtures, return unknown when authority is unclear, and never execute argv.\n"
	test := fmt.Sprintf("#!/bin/sh\nset -eu\nroot=$(CDPATH= cd -- \"$(dirname -- \"$0\")\" && pwd)\nmeta=$(printf '{\"protocol\":1,\"id\":\"%s\",\"mode\":\"metadata\"}\\n' | \"$root/plugin.sh\")\nprintf '%%s\\n' \"$meta\" | grep -F 'current' >/dev/null\nprintf '{\"protocol\":1,\"id\":\"%s\",\"mode\":\"current\"}\\n' | \"$root/plugin.sh\" >/dev/null\nprintf '{\"protocol\":1,\"id\":\"%s\",\"mode\":\"refresh\"}\\n' | \"$root/plugin.sh\" >/dev/null\n", o.ID, o.ID, o.ID)
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
func has(v []string, w string) bool {
	for _, x := range v {
		if x == w {
			return true
		}
	}
	return false
}
func quote(s string) string      { b, _ := json.Marshal(s); return string(b) }
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
