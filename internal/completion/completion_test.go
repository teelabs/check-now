package completion

import (
	"strings"
	"testing"
)

func TestNormalizeShell(t *testing.T) {
	for _, tc := range []struct {
		name, want string
	}{
		{"bash", "bash"}, {"zsh", "zsh"}, {"fish", "fish"},
		{"powershell", "powershell"}, {"pwsh", "powershell"},
	} {
		got, err := NormalizeShell(tc.name)
		if err != nil || got != tc.want {
			t.Fatalf("NormalizeShell(%q) = %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := NormalizeShell("dash"); err == nil || !strings.Contains(err.Error(), "bash") {
		t.Fatalf("unsupported shell error = %v", err)
	}
}

func TestDetectShell(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"unix SHELL", map[string]string{"SHELL": "/usr/bin/zsh"}, "zsh"},
		{"pwsh fallback", map[string]string{"PSModulePath": "/x"}, "powershell"},
		{"pwsh wins over unknown SHELL", map[string]string{"SHELL": "/bin/unknown", "PSModulePath": "/x"}, "powershell"},
	} {
		got, err := DetectShell(tc.env)
		if err != nil || got != tc.want {
			t.Fatalf("%s: DetectShell = %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
	for _, env := range []map[string]string{{}, {"SHELL": "/bin/dash"}} {
		if _, err := DetectShell(env); err == nil || !strings.Contains(err.Error(), "supported") {
			t.Fatalf("DetectShell(%v) error = %v", env, err)
		}
	}
}

func TestScriptsContainPublicCommandsAndStaticCandidates(t *testing.T) {
	for _, shell := range SupportedShells() {
		got, err := Script(shell)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"shell", "refresh", "dismiss", "list", "doctor", "plugin", "completion", "--force", "node", "bun", "deno", "fnm", "codex"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s completion lacks %q", shell, want)
			}
		}
	}
}

func TestScriptRejectsUnknownShell(t *testing.T) {
	if _, err := Script("dash"); err == nil {
		t.Fatal("Script accepted unsupported shell")
	}
}

func TestZshScriptDeclaresCommandForCompinit(t *testing.T) {
	got, err := Script("zsh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "#compdef check-now") {
		t.Fatal("zsh completion must declare check-now for compinit to register it")
	}
}
