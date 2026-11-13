package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"check-now/internal/scaffold"
)

func TestHelpEntrypointsAndCommandPages(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}, {"help", "--help"}} {
		stdout, stderr, code := runCLI(t, args...)
		if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage\n") || !strings.Contains(stdout, "Global options") {
			t.Fatalf("args %v: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
	for _, args := range [][]string{
		{"help", "version"}, {"version", "--help"},
		{"help", "shell"}, {"shell", "--help"},
		{"help", "refresh"}, {"refresh", "--help"},
		{"help", "dismiss"}, {"dismiss", "--help"},
		{"help", "list"}, {"list", "--help"},
		{"help", "doctor"}, {"doctor", "--help"},
		{"help", "plugin", "create"}, {"plugin", "create", "--help"},
		{"help", "completion"}, {"completion", "--help"},
		{"help", "help"},
	} {
		stdout, stderr, code := runCLI(t, args...)
		if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage\n") || !strings.Contains(stdout, "Example") {
			t.Fatalf("args %v: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
	stdout, _, _ := runCLI(t)
	if strings.Contains(stdout, "--background") {
		t.Fatalf("internal refresh option exposed in help: %s", stdout)
	}
	stdout, _, _ = runCLI(t, "help", "plugin", "create")
	for _, option := range []string{"--id", "--name", "--description", "--source-url", "--non-interactive"} {
		if !strings.Contains(stdout, option) {
			t.Errorf("plugin create help missing %s", option)
		}
	}
}

func TestHelpRunsBeforePluginDiscoveryAndErrorsAreConcise(t *testing.T) {
	pluginRoot := t.TempDir()
	pluginDir := filepath.Join(pluginRoot, "marker")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pluginPath := filepath.Join(pluginDir, "plugin.sh")
	if err := os.WriteFile(pluginPath, []byte("#!/bin/sh\necho plugin-discovery-ran >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCLIWithPluginRoot(t, pluginRoot, "--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "check-now —") {
		t.Fatalf("help consulted discovery or failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, args := range [][]string{{"unknown"}, {"shell", "extra"}, {"doctor", "--force"}, {"refresh", "--bad"}, {"help", "plugin", "unknown"}, {"completion", "unknown"}, {"plugin", "create", "--not-a-flag"}} {
		_, stderr, code = runCLI(t, args...)
		if code != 2 || !strings.Contains(stderr, "Run 'check-now help'") || strings.Contains(stderr, "usage:") {
			t.Errorf("args %v: code=%d stderr=%q", args, code, stderr)
		}
	}
}

func TestRefreshBackgroundRemainsAcceptedButPrivate(t *testing.T) {
	if err := validateCommandArgs([]string{"refresh", "--background"}); err != nil {
		t.Fatalf("internal refresh invocation rejected: %v", err)
	}
	if strings.Contains(renderHelp("", false), "--background") {
		t.Fatal("internal refresh option appears in public help")
	}
}

func TestDoctorDurationRejectsOverflowingHourCount(t *testing.T) {
	for _, value := range []string{"6000000", "-10000000"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("CHECK_NOW_STALE_INTERVAL", value)
			got, finding := doctorDuration("CHECK_NOW_STALE_INTERVAL", time.Second)
			if finding.Status != "error" || got != time.Second {
				t.Fatalf("duration=%s finding=%+v; want fallback and error", got, finding)
			}
		})
	}
}

func TestDoctorReportsActionableFindingsWithoutRefreshOrEnvironmentValues(t *testing.T) {
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "plugins-root-sensitive")
	pluginDir := filepath.Join(pluginRoot, "tool")
	cacheHome := filepath.Join(root, "cache-home-sensitive")
	cacheDir := filepath.Join(cacheHome, "check-now")
	marker := filepath.Join(root, "refresh-marker")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	plugin := `#!/bin/sh
request=$(cat)
case "$request" in
  *'"mode":"metadata"'*) printf '%s\n' '{"protocol":2,"id":"tool","name":"Tool","description":"Checks Tool","runtime":"shell","capabilities":["current","refresh","health"]}' ;;
  *'"mode":"health"'*) printf '%s\n' '{"protocol":2,"id":"tool","checked_at":"2026-01-01T00:00:00Z","checks":[{"code":"config.ready","status":"ok","summary":"Local configuration is ready."}]}' ;;
  *'"mode":"current"'*) printf '%s\n' '{"protocol":2,"id":"tool","tool":"Tool","status":"up_to_date","current_version":"1.0.0","checked_at":"2026-01-01T00:00:00Z"}' ;;
  *'"mode":"refresh"'*) touch ` + "'" + marker + "'" + ` ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.sh"), []byte(plugin), 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCLIWithEnv(t, map[string]string{
		"CHECK_NOW_BUILTIN_PLUGIN_DIR": pluginRoot,
		"CHECK_NOW_PLUGIN_DIRS":        "",
		"CHECK_NOW_PLUGIN_TIMEOUT":     "sensitive-invalid-duration",
		"CHECK_NOW_STALE_INTERVAL":     "0s",
		"XDG_CACHE_HOME":               cacheHome,
		"CHECK_NOW_DOCTOR_SENTINEL":    "sensitive-environment-value",
	}, "doctor")
	if code != 0 || stderr != "" {
		t.Fatalf("doctor code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, want := range []string{"plugin.tool.config.ready", "cache.tool.target", "settings.plugin_timeout", "Suggested repair", "cache.writable"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("doctor output missing %q: %s", want, stdout)
		}
	}
	for _, forbidden := range []string{pluginRoot, cacheHome, "sensitive-environment-value", "sensitive-invalid-duration", "0s", "Overall", "grade"} {
		if strings.Contains(stdout, forbidden) {
			t.Errorf("doctor output includes %q: %s", forbidden, stdout)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("doctor invoked refresh mode: stat error=%v", err)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cache probe left files behind: %v", entries)
	}
}

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	return runCLIWithPluginRoot(t, filepath.Join(t.TempDir(), "does-not-exist"), args...)
}

func runCLIWithPluginRoot(t *testing.T, pluginRoot string, args ...string) (string, string, int) {
	t.Helper()
	return runCLIWithEnv(t, map[string]string{"CHECK_NOW_BUILTIN_PLUGIN_DIR": pluginRoot}, args...)
}

func runCLIWithEnv(t *testing.T, overrides map[string]string, args ...string) (string, string, int) {
	t.Helper()
	cmdArgs := []string{"-test.run=^TestCLIChild$"}
	cmd := exec.Command(os.Args[0], cmdArgs...)
	values := map[string]string{"CHECK_NOW_HELP_CHILD": "1", "CHECK_NOW_HELP_ARGS": strings.Join(args, "\n")}
	for name, value := range overrides {
		values[name] = value
	}
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, replaced := values[name]; !replaced {
				cmd.Env = append(cmd.Env, entry)
			}
		}
	}
	for name, value := range values {
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return stdout.String(), stderr.String(), exit.ExitCode()
	}
	t.Fatalf("run CLI: %v", err)
	return "", "", -1
}

func TestCLIChild(t *testing.T) {
	if os.Getenv("CHECK_NOW_HELP_CHILD") != "1" {
		return
	}
	os.Args = []string{"check-now"}
	if args := os.Getenv("CHECK_NOW_HELP_ARGS"); args != "" {
		os.Args = append(os.Args, strings.Split(args, "\n")...)
	}
	main()
	os.Exit(0)
}

func TestCompletionCommandSelectsAndRejectsShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell", "pwsh"} {
		if err := completionCommand([]string{shell}); err != nil {
			t.Fatalf("completion %s: %v", shell, err)
		}
	}
	if err := completionCommand([]string{"bash", "extra"}); err == nil {
		t.Fatal("completion accepted an extra argument")
	}
}

func TestPromptPluginExplainsOptionalFields(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	p := scaffold.Options{Runtime: "shell"}
	promptPlugin(&p, new(string), bufio.NewReader(strings.NewReader("id\nName\nDescription\n\n\n\n\n")))
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if !bytes.Contains(out, []byte("optional")) || !bytes.Contains(out, []byte("local command")) || !bytes.Contains(out, []byte("source URL")) {
		t.Fatalf("wizard output lacks short field explanations: %s", out)
	}
}
