// Package completion provides static, side-effect-free shell completion scripts.
package completion

import (
	"fmt"
	"path/filepath"
	"strings"
)

var supported = []string{"bash", "zsh", "fish", "powershell"}

// SupportedShells returns the canonical shell names in stable display order.
func SupportedShells() []string { return append([]string(nil), supported...) }

// NormalizeShell accepts canonical names and the PowerShell alias pwsh.
func NormalizeShell(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(filepath.Base(name)))
	if name == "pwsh" {
		name = "powershell"
	}
	for _, shell := range supported {
		if shell == name {
			return name, nil
		}
	}
	return "", fmt.Errorf("unsupported shell %q; supported shells: %s", name, strings.Join(supported, ", "))
}

// DetectShell uses SHELL on Unix and PSModulePath as a PowerShell fallback.
func DetectShell(env map[string]string) (string, error) {
	if shell := strings.TrimSpace(env["SHELL"]); shell != "" {
		if normalized, err := NormalizeShell(shell); err == nil {
			return normalized, nil
		}
	}
	if strings.TrimSpace(env["PSModulePath"]) != "" {
		return "powershell", nil
	}
	return "", fmt.Errorf("could not detect a supported shell; supported shells: %s", strings.Join(supported, ", "))
}

// Script returns one embedded script and performs no runtime discovery or cache access.
func Script(shell string) (string, error) {
	normalized, err := NormalizeShell(shell)
	if err != nil {
		return "", err
	}
	script, ok := scriptData[normalized]
	if !ok {
		return "", fmt.Errorf("completion script unavailable for %q", normalized)
	}
	return script, nil
}
