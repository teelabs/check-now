package main

import (
	"check-now/internal/app"
	"check-now/internal/cache"
	"check-now/internal/discovery"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func doctorCommand() {
	findings := make([]app.Finding, 0)
	configuredRoots := app.ConfiguredPlugins()
	for index, root := range configuredRoots {
		findings = append(findings, checkPluginRoot(fmt.Sprintf("plugins.configured.%d", index+1), root))
	}

	exe, _ := os.Executable()
	builtinOverride := os.Getenv("CHECK_NOW_BUILTIN_PLUGIN_DIR")
	var builtinRoots []string
	if builtinOverride != "" {
		builtinRoots = []string{builtinOverride}
		findings = append(findings, checkPluginRoot("plugins.builtin", builtinOverride))
	} else {
		builtinRoots = discovery.BuiltinDirectories(exe)
		available := false
		for _, root := range builtinRoots {
			if directoryReadable(root) {
				available = true
				break
			}
		}
		if !available {
			findings = append(findings, app.Finding{
				Component: "plugins.builtin", Status: "warning",
				Summary: "No built-in plugin directory is available.",
				Repair:  "Reinstall the application with its built-in plugins.",
			})
		}
	}

	stale, staleFinding := doctorDuration("CHECK_NOW_STALE_INTERVAL", 24*time.Hour)
	findings = append(findings, staleFinding)
	timeout, timeoutFinding := doctorDuration("CHECK_NOW_PLUGIN_TIMEOUT", 5*time.Second)
	findings = append(findings, timeoutFinding)

	plugins := discovery.Find("", append(configuredRoots, builtinRoots...))
	store := cache.New(app.DefaultCache(), stale)
	findings = append(findings, app.Diagnose(context.Background(), plugins, store, timeout)...)
	app.WriteFindings(os.Stdout, findings)
}

func checkPluginRoot(component, root string) app.Finding {
	if !directoryReadable(root) {
		return app.Finding{
			Component: component, Status: "warning",
			Summary: "A configured plugin root is missing or unreadable.",
			Repair:  "Correct CHECK_NOW_PLUGIN_DIRS or CHECK_NOW_BUILTIN_PLUGIN_DIR and verify directory permissions.",
		}
	}
	return app.Finding{Component: component, Status: "ok", Summary: "A configured plugin root is readable."}
}

func directoryReadable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	_, err = os.ReadDir(path)
	return err == nil
}

func doctorDuration(name string, fallback time.Duration) (time.Duration, app.Finding) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, app.Finding{Component: "settings." + strings.ToLower(strings.TrimPrefix(name, "CHECK_NOW_")), Status: "ok", Summary: "The setting uses its documented default."}
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		if hours, numberErr := strconv.Atoi(value); numberErr == nil {
			if hours < 0 {
				return fallback, app.Finding{Component: "settings." + strings.ToLower(strings.TrimPrefix(name, "CHECK_NOW_")), Status: "error", Summary: "The duration setting must be positive.", Repair: "Set the setting to a positive duration such as 5s or 24h."}
			}
			maxHours := int64(^uint64(0)>>1) / int64(time.Hour)
			if int64(hours) > maxHours {
				return fallback, app.Finding{Component: "settings." + strings.ToLower(strings.TrimPrefix(name, "CHECK_NOW_")), Status: "error", Summary: "The duration setting is too large.", Repair: "Set the setting to a positive duration such as 5s or 24h."}
			}
			duration = time.Duration(hours) * time.Hour
		} else {
			return fallback, app.Finding{Component: "settings." + strings.ToLower(strings.TrimPrefix(name, "CHECK_NOW_")), Status: "error", Summary: "The duration setting is invalid.", Repair: "Set the setting to a positive duration such as 5s or 24h."}
		}
	}
	if duration <= 0 {
		return fallback, app.Finding{Component: "settings." + strings.ToLower(strings.TrimPrefix(name, "CHECK_NOW_")), Status: "error", Summary: "The duration setting must be positive.", Repair: "Set the setting to a positive duration such as 5s or 24h."}
	}
	return duration, app.Finding{Component: "settings." + strings.ToLower(strings.TrimPrefix(name, "CHECK_NOW_")), Status: "ok", Summary: "The duration setting is valid."}
}
