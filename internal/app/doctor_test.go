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
	var report strings.Builder
	WriteFindings(&report, findings)
	if strings.Contains(report.String(), "private-environment-secret") {
		t.Fatal("plugin stderr leaked into doctor output")
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
