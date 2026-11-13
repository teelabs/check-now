package main

import (
	"check-now/internal/app"
	"strings"
	"testing"
)

func TestRenderDoctorGroupsFindingsAndUsesSemanticColors(t *testing.T) {
	findings := []app.Finding{
		{Component: "settings.plugin_timeout", Status: "ok", Summary: "The setting uses its documented default."},
		{Component: "plugins.builtin", Status: "warning", Summary: "No built-in plugin directory is available."},
		{Component: "cache.writable", Status: "ok", Summary: "The cache directory is writable."},
		{Component: "plugin.bun.command.available", Plugin: "bun", Status: "ok", Summary: "The command is available."},
		{Component: "plugin.bun.current", Plugin: "bun", Status: "warning", Summary: "The local version could not be confirmed.", Repair: "Install the tool."},
		{Component: "cache.bun.target", Plugin: "bun", Status: "error", Summary: "The target cache record is corrupt."},
		{Component: "cache.bun.refresh", Plugin: "bun", Status: "not_checked", Summary: "No refresh attempt has been recorded."},
	}
	var colored strings.Builder
	renderDoctor(&colored, findings, true)
	var plain strings.Builder
	renderDoctor(&plain, findings, false)
	text := plain.String()
	for _, want := range []string{
		"check-now doctor", "Configuration", "Settings", "Plugin roots", "Adapters", "Cache",
		"bun", "command.available", "current", "writable", "target", "refresh",
		"Suggested repair: Install the tool.", "[NOT CHECKED]",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("doctor report missing %q:\n%s", want, text)
		}
	}
	for _, want := range []string{
		"\x1b[38;2;88;166;255m", "\x1b[38;2;63;185;80m",
		"\x1b[38;2;210;153;34m", "\x1b[38;2;248;81;73m",
		"\x1b[38;2;210;168;255m", "\x1b[38;2;139;148;158m",
	} {
		if !strings.Contains(colored.String(), want) {
			t.Errorf("terminal report missing palette color %q:\n%s", want, colored.String())
		}
	}
	configuration := strings.Index(text, "Configuration")
	adapters := strings.Index(text, "Adapters")
	cache := strings.Index(text, "Cache")
	if !(configuration < adapters && adapters < cache) {
		t.Fatalf("sections are not ordered configuration, adapters, cache:\n%s", text)
	}
	if strings.Contains(text, "Overall") || strings.Contains(text, "grade") {
		t.Fatalf("doctor report added an aggregate grade:\n%s", text)
	}
}

func TestRenderDoctorKeepsRedirectedOutputUncolored(t *testing.T) {
	findings := []app.Finding{
		{Component: "settings.plugin_timeout", Status: "ok", Summary: "The setting uses its documented default."},
		{Component: "plugin.bun.current", Plugin: "bun", Status: "ok", Summary: "The local probe completed."},
	}
	var output strings.Builder
	renderDoctor(&output, findings, false)
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("redirected report contains ANSI escape sequences: %q", output.String())
	}
}
