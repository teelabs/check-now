package app

import (
	"check-now/internal/cache"
	"check-now/internal/protocol"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiagnoseUsesOnlyLocalModesAndDoesNotChangeCacheRecords(t *testing.T) {
	root := t.TempDir()
	plugin := filepath.Join(root, "tool", "plugin.sh")
	refreshMarker := filepath.Join(root, "refresh-called")
	pluginBody := `#!/bin/sh
request=$(cat)
mode=$(printf '%s' "$request" | sed -n 's/.*"mode":"\([^"]*\)".*/\1/p')
case "$mode" in
  metadata) printf '%s\n' '{"protocol":2,"id":"tool","name":"Tool","description":"Checks Tool","runtime":"shell","capabilities":["current","refresh","health"]}' ;;
  health) printf '%s\n' '{"protocol":2,"id":"tool","checked_at":"2026-01-01T00:00:00Z","checks":[{"code":"config.ready","status":"ok","summary":"Adapter configuration is ready."}]}' ;;
  current) printf '%s\n' '{"protocol":2,"id":"tool","tool":"Tool","status":"up_to_date","current_version":"1.0.0","checked_at":"2026-01-01T00:00:00Z"}' ;;
  refresh) echo called > ` + "'" + refreshMarker + "'" + `; exit 4 ;;
  *) exit 2 ;;
esac
`
	if err := os.MkdirAll(filepath.Dir(plugin), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plugin, []byte(pluginBody), 0755); err != nil {
		t.Fatal(err)
	}
	store := cache.New(filepath.Join(root, "cache"), time.Hour)
	if err := store.WriteTarget(protocol.Result{Protocol: protocol.Version, ID: "tool", Tool: "Tool", Status: "up_to_date", CheckedAt: time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteCurrent(protocol.Result{Protocol: protocol.Version, ID: "tool", Tool: "Tool", Status: "up_to_date", CurrentVersion: "1.0.0", CheckedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteControl("tool", cache.Control{Dismissed: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteRefresh("tool", cache.RefreshRecord{CheckedAt: time.Now().UTC().Format(time.RFC3339), Outcome: "process_failure"}); err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, store.Dir)
	findings := Diagnose(context.Background(), []string{plugin}, store, time.Second)
	after := snapshotFiles(t, store.Dir)
	if len(before) != len(after) {
		t.Fatalf("cache file count changed: before=%v after=%v", before, after)
	}
	for path, body := range before {
		if after[path] != body {
			t.Fatalf("cache record changed during diagnosis: %s", path)
		}
	}
	if _, err := os.Stat(refreshMarker); !os.IsNotExist(err) {
		t.Fatalf("doctor invoked refresh mode: stat error=%v", err)
	}
	target, ok := findFinding(findings, "cache.tool.target")
	if !ok || target.Status != "warning" || !strings.Contains(target.Summary, "process_failure") {
		t.Fatalf("target finding=%+v ok=%v", target, ok)
	}
	if _, ok := findFinding(findings, "plugin.tool.config.ready"); !ok {
		t.Fatalf("adapter health finding missing: %+v", findings)
	}
	if _, ok := findFinding(findings, "plugin.tool.current"); !ok {
		t.Fatalf("current probe finding missing: %+v", findings)
	}
}

func TestDiagnoseReportsCorruptCacheWithoutExposingPluginStderr(t *testing.T) {
	root := t.TempDir()
	plugin := filepath.Join(root, "tool", "plugin.sh")
	if err := os.MkdirAll(filepath.Dir(plugin), 0755); err != nil {
		t.Fatal(err)
	}
	body := `#!/bin/sh
request=$(cat)
case "$request" in
  *'"mode":"metadata"'*) printf '%s\n' '{"protocol":2,"id":"tool","name":"Tool","description":"Checks Tool","runtime":"shell","capabilities":["current","refresh","health"]}' ;;
  *'"mode":"health"'*) echo private-environment-secret >&2; exit 2 ;;
  *) printf '%s\n' '{"protocol":2,"id":"tool","tool":"Tool","status":"up_to_date","checked_at":"2026-01-01T00:00:00Z"}' ;;
esac
`
	if err := os.WriteFile(plugin, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	store := cache.New(filepath.Join(root, "cache"), time.Hour)
	if err := os.MkdirAll(filepath.Dir(store.TargetPath("tool")), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.TargetPath("tool"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	findings := Diagnose(context.Background(), []string{plugin}, store, time.Second)
	corrupt, ok := findFinding(findings, "cache.tool.target")
	if !ok || corrupt.Status != "error" || corrupt.Repair == "" {
		t.Fatalf("target finding=%+v ok=%v", corrupt, ok)
	}
	for _, finding := range findings {
		if strings.Contains(finding.Component+finding.Plugin+finding.Summary+finding.Repair, "private-environment-secret") {
			t.Fatalf("plugin stderr leaked into doctor finding: %+v", finding)
		}
	}
}

func TestDiagnoseClassifiesCacheRecordStates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, store cache.Store)
		want  map[string]string
	}{
		{
			name: "missing",
			want: map[string]string{
				"cache.tool.target": "warning", "cache.tool.current": "warning",
				"cache.tool.control": "ok", "cache.tool.refresh": "not_checked",
			},
		},
		{
			name: "stale",
			setup: func(t *testing.T, store cache.Store) {
				old := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
				if err := store.WriteTarget(protocol.Result{Protocol: protocol.Version, ID: "tool", Tool: "Tool", Status: "unknown", CheckedAt: old}); err != nil {
					t.Fatal(err)
				}
				if err := store.WriteCurrent(protocol.Result{Protocol: protocol.Version, ID: "tool", Tool: "Tool", Status: "up_to_date", CurrentVersion: "1.0.0", CheckedAt: old}); err != nil {
					t.Fatal(err)
				}
				if err := store.WriteRefresh("tool", cache.RefreshRecord{CheckedAt: time.Now().UTC().Format(time.RFC3339), Outcome: "success"}); err != nil {
					t.Fatal(err)
				}
			},
			want: map[string]string{
				"cache.tool.target": "warning", "cache.tool.current": "warning",
				"cache.tool.control": "ok", "cache.tool.refresh": "ok",
			},
		},
		{
			name: "corrupt",
			setup: func(t *testing.T, store cache.Store) {
				for _, path := range []string{store.TargetPath("tool"), store.CurrentPath("tool"), store.ControlPath("tool"), store.RefreshPath("tool")} {
					if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			},
			want: map[string]string{
				"cache.tool.target": "error", "cache.tool.current": "error",
				"cache.tool.control": "warning", "cache.tool.refresh": "warning",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			plugin := writeHealthyDoctorPlugin(t, root)
			store := cache.New(filepath.Join(root, "cache"), time.Hour)
			if tc.setup != nil {
				tc.setup(t, store)
			}
			findings := Diagnose(context.Background(), []string{plugin}, store, time.Second)
			for component, status := range tc.want {
				finding, ok := findFinding(findings, component)
				if !ok || finding.Status != status {
					t.Errorf("%s finding=%+v ok=%v; want status %s", component, finding, ok, status)
				}
			}
		})
	}
}

func TestDiagnoseTreatsMissingRefreshHistoryAsNotChecked(t *testing.T) {
	root := t.TempDir()
	plugin := writeHealthyDoctorPlugin(t, root)
	store := cache.New(filepath.Join(root, "cache"), time.Hour)
	findings := Diagnose(context.Background(), []string{plugin}, store, time.Second)
	finding, ok := findFinding(findings, "cache.tool.refresh")
	if !ok || finding.Status != "not_checked" || finding.Repair != "" {
		t.Fatalf("refresh finding=%+v ok=%v; want neutral not_checked without repair", finding, ok)
	}
}

func writeHealthyDoctorPlugin(t *testing.T, root string) string {
	t.Helper()
	plugin := filepath.Join(root, "tool", "plugin.sh")
	if err := os.MkdirAll(filepath.Dir(plugin), 0755); err != nil {
		t.Fatal(err)
	}
	body := `#!/bin/sh
request=$(cat)
case "$request" in
  *'"mode":"metadata"'*) printf '%s\n' '{"protocol":2,"id":"tool","name":"Tool","description":"Checks Tool","runtime":"shell","capabilities":["current","refresh","health"]}' ;;
  *'"mode":"health"'*) printf '%s\n' '{"protocol":2,"id":"tool","checked_at":"2026-01-01T00:00:00Z","checks":[{"code":"config.ready","status":"ok","summary":"Configuration is ready."}]}' ;;
  *'"mode":"current"'*) printf '%s\n' '{"protocol":2,"id":"tool","tool":"Tool","status":"up_to_date","current_version":"1.0.0","checked_at":"2026-01-01T00:00:00Z"}' ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(plugin, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	return plugin
}

func TestDiagnoseRedactsEnvironmentValuesAtTokenBoundaries(t *testing.T) {
	const secret = "doctor-private-environment-sentinel"
	const shortSecret = "Z"
	t.Setenv("CHECK_NOW_DOCTOR_TEST_SECRET", secret)
	t.Setenv("CHECK_NOW_DOCTOR_SHORT_SECRET", shortSecret)
	root := t.TempDir()
	plugin := writeHealthyDoctorPlugin(t, root)
	pluginDir := filepath.Join(root, secret)
	if err := os.Rename(filepath.Dir(plugin), pluginDir); err != nil {
		t.Fatal(err)
	}
	plugin = filepath.Join(pluginDir, "plugin.sh")
	body, err := os.ReadFile(plugin)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(string(body), `"id":"tool"`, `"id":"`+secret+`"`))
	replacement := `"code":"` + secret + `","status":"ok","summary":"Found ` + secret + ` at the release endpoint; prefix` + shortSecret + `suffix, prefix` + shortSecret + shortSecret + `., and standalone ` + shortSecret + ` value.","repair":"Unset ` + secret + ` and preserve prefix` + shortSecret + `suffix; remove standalone ` + shortSecret + ` value."`
	body = []byte(strings.Replace(string(body), `"code":"config.ready","status":"ok","summary":"Configuration is ready."`, replacement, 1))
	if err := os.WriteFile(plugin, body, 0755); err != nil {
		t.Fatal(err)
	}
	findings := Diagnose(context.Background(), []string{plugin}, cache.New(filepath.Join(root, "cache"), time.Hour), time.Second)
	var report strings.Builder
	for _, finding := range findings {
		report.WriteString(finding.Component + finding.Plugin + finding.Summary + finding.Repair)
	}
	if strings.Contains(report.String(), secret) {
		t.Fatalf("doctor findings exposed a complete environment value: %s", report.String())
	}
	if !strings.Contains(report.String(), "prefixZsuffix") {
		t.Fatalf("doctor corrupted an embedded environment substring: %s", report.String())
	}
	if !strings.Contains(report.String(), "prefixZZ.") {
		t.Fatalf("doctor corrupted repeated embedded environment substrings: %s", report.String())
	}
	if !strings.Contains(report.String(), "release endpoint") {
		t.Fatalf("doctor corrupted ordinary adapter wording: %s", report.String())
	}
	if strings.Contains(report.String(), "standalone Z value") {
		t.Fatalf("doctor did not redact a complete short environment value: %s", report.String())
	}
}

func snapshotFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[relative] = string(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func findFinding(findings []Finding, component string) (Finding, bool) {
	for _, finding := range findings {
		if finding.Component == component {
			return finding, true
		}
	}
	return Finding{}, false
}
