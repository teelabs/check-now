package app

import (
	"check-now/internal/cache"
	"check-now/internal/protocol"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRefreshPreservesPreviousCacheWhenPluginReturnsUnknown(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "tool")
	plugin := filepath.Join(pluginDir, "plugin.sh")
	body := "#!/bin/sh\ncat >/dev/null\nprintf '%s\\n' '{\"protocol\":1,\"id\":\"tool\",\"tool\":\"Tool\",\"status\":\"unknown\",\"checked_at\":\"2026-01-01T00:00:00Z\"}'\n"
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plugin, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(root, "cache")
	store := cache.New(cacheDir, time.Hour)
	if err := store.Write(protocol.Result{Protocol: 1, ID: "tool", Tool: "Tool", Status: "up_to_date", CheckedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	if err := (New([]string{plugin}, cacheDir, time.Hour, time.Second)).Refresh(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	got, ok := store.ReadAny("tool")
	if !ok || got.Status != "up_to_date" {
		t.Fatalf("cache=%+v ok=%v", got, ok)
	}
}

func TestFormatUpdatesRendersReadableTable(t *testing.T) {
	got := formatUpdates([]protocol.Result{{
		Tool:           "Node",
		Status:         "update_available",
		CurrentVersion: "24.19.0",
		Updates: []protocol.Update{
			{Scope: "same_major", Track: "24.x", Version: "24.20.0", Argv: []string{"fnm", "install", "24.20.0"}, SourceURL: "https://nodejs.org/en/download"},
			{Scope: "latest", Track: "latest", Version: "26.12.1", Argv: []string{"fnm", "install", "26.12.1"}, SourceURL: "https://nodejs.org/en/download"},
		},
	}})
	for _, want := range []string{"Updates available!", "┌", "├", "┼", "└", "┴", "│ Name", "Version", "Command to update", "Source", "Node", "24.20.0 (24.x)", "fnm install 26.12.1", "https://nodejs.org/en/download"} {
		if !strings.Contains(got, want) {
			t.Fatalf("table missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "update_available") {
		t.Fatalf("machine status leaked into UI:\n%s", got)
	}
	var pipePositions string
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "│") && strings.Count(line, "│") > 2 {
			var positions []int
			for i, r := range line {
				if r == '|' || r == '+' {
					positions = append(positions, i)
				}
			}
			gotPositions := fmt.Sprint(positions)
			if pipePositions == "" {
				pipePositions = gotPositions
			} else if gotPositions != pipePositions {
				t.Fatalf("unaligned table row %q: %s != %s", line, gotPositions, pipePositions)
			}
		}
	}
	if pipePositions == "" {
		t.Fatalf("expected aligned table lines:\n%s", got)
	}
	if strings.Count(got, "Node") != 1 || strings.Count(got, "https://nodejs.org/en/download") != 1 {
		t.Fatalf("group labels should not repeat:\n%s", got)
	}
	titleLine := ""
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "Updates available!") {
			titleLine = line
			break
		}
	}
	if titleLine == "" || strings.Index(titleLine, "Updates available!") <= 1 {
		t.Fatalf("title is not centered: %q", titleLine)
	}
	lines := strings.Split(got, "\n")
	if len(lines) < 5 || strings.Contains(lines[0], "┬") || !strings.Contains(lines[2], "┬") || strings.Contains(lines[2], "┼") || !strings.Contains(lines[4], "┼") {
		t.Fatalf("title must occupy one continuous top cell:\n%s", got)
	}
	dataRows := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "│") && strings.Count(line, "│") > 2 {
			dataRows++
		}
	}
	if dataRows != 4 {
		t.Fatalf("expected header plus three grouped rows, got %d:\n%s", dataRows, got)
	}
}
