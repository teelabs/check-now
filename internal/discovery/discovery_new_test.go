package discovery

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFindFolderPluginsUsesDirectoryIDAndMetadata(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "nodejs")
	if err := os.Mkdir(p, 0755); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(p, "plugin.sh")
	if err := os.WriteFile(plugin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	got := Find("", []string{root})
	if len(got) != 1 || ID(got[0]) != "nodejs" || got[0] != plugin {
		t.Fatalf("got=%v id=%q", got, ID(got[0]))
	}
}

func TestBuiltinsOwnTheirPluginImplementation(t *testing.T) {
	root := filepath.Join("..", "..", "plugins")
	for _, id := range []string{"node", "bun", "deno", "fnm", "codex"} {
		data, err := os.ReadFile(filepath.Join(root, id, "plugin.sh"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) == "" || len(data) < 200 {
			t.Fatalf("%s plugin is unexpectedly a wrapper", id)
		}
		if filepath.Base(string(data)) == "check-now-tool.sh" || bytes.Contains(data, []byte("../check-now-tool.sh")) {
			t.Fatalf("%s delegates implementation to check-now-tool.sh", id)
		}
	}
}

func TestFindUsesFirstPluginForDuplicateCanonicalID(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	for _, root := range []string{first, second} {
		p := filepath.Join(root, "same", "plugin.sh")
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	got := Find("", []string{first, second})
	if len(got) != 1 || filepath.Dir(filepath.Dir(got[0])) != first {
		t.Fatalf("got=%v", got)
	}
}
