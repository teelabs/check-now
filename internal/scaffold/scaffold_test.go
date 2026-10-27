package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"check-now/internal/protocol"
)

func TestCreateNonInteractiveGeneratesPluginBundle(t *testing.T) {
	root := t.TempDir()
	got, err := Create(root, Options{ID: "nodejs", Name: "Node.js", Description: "Checks Node.js installations", Tool: "node", SourceURL: "https://nodejs.org", Runtime: "shell", Manager: "fnm", Capabilities: []string{"refresh"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "nodejs") {
		t.Fatalf("path=%q", got)
	}
	for _, name := range []string{"plugin.sh", "README.md", "AGENTS.md", "test.sh"} {
		if _, err := os.Stat(filepath.Join(got, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	st, err := os.Stat(filepath.Join(got, "plugin.sh"))
	if err != nil || st.Mode()&0111 == 0 {
		t.Fatalf("plugin.sh is not executable: %v mode=%v", err, st.Mode())
	}
	data, err := os.ReadFile(filepath.Join(got, "plugin.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "TODO") || !strings.Contains(string(data), "metadata") {
		t.Fatal("generated plugin lacks implementation guidance")
	}
	smoke := exec.Command(filepath.Join(got, "test.sh"))
	if out, err := smoke.CombinedOutput(); err != nil {
		t.Fatalf("generated smoke test: %v: %s", err, out)
	}
	cmd := exec.Command(filepath.Join(got, "plugin.sh"))
	cmd.Stdin = strings.NewReader(`{"protocol":1,"id":"nodejs","mode":"metadata"}` + "\n")
	metaOutput, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	meta, err := protocol.DecodeMetadata(metaOutput)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID != "nodejs" || meta.Manager != "fnm" || meta.SourceURL != "https://nodejs.org" {
		t.Fatalf("metadata=%+v", meta)
	}
}

func TestCreateRejectsMissingRequiredAndExistingWithoutForce(t *testing.T) {
	root := t.TempDir()
	if _, err := Create(root, Options{ID: "bad id", Name: "Name", Description: "Description"}); err == nil {
		t.Fatal("invalid id accepted")
	}
	if _, err := Create(root, Options{ID: "tool", Name: "Tool", Description: "Description"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, Options{ID: "tool", Name: "Tool", Description: "Description"}); err == nil {
		t.Fatal("existing directory accepted")
	}
	if _, err := Create(root, Options{ID: "tool", Name: "Tool", Description: "Description", Force: true}); err != nil {
		t.Fatal(err)
	}
}
