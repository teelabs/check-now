package app

import (
	"bytes"
	"check-now/internal/cache"
	"check-now/internal/discovery"
	"check-now/internal/protocol"
	"check-now/internal/runner"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Finding struct {
	Component string
	Status    string
	Summary   string
	Repair    string
}

// Diagnose performs local adapter checks and cache reads. It never calls the
// adapter's refresh mode and never writes cache records.
func Diagnose(ctx context.Context, plugins []string, store cache.Store, timeout time.Duration) []Finding {
	sanitizeAdapterText := newAdapterTextSanitizer()
	findings := []Finding{probeCache(store.Dir)}
	if len(plugins) == 0 {
		findings = append(findings, Finding{
			Component: "plugins.discovery", Status: "warning",
			Summary: "No executable adapters were discovered.",
			Repair:  "Check the configured plugin roots and install at least one adapter.",
		})
	}
	for _, plugin := range plugins {
		id := sanitizeAdapterText(discovery.ID(plugin))
		base := runner.Runner{Timeout: timeout}
		_, err := base.Metadata(ctx, plugin, discovery.ID(plugin))
		if err != nil {
			findings = append(findings, Finding{
				Component: "plugin." + id + ".metadata", Status: "error",
				Summary: "Adapter metadata is invalid or unavailable.",
				Repair:  "Update the adapter to protocol v2 and advertise current, refresh, and health capabilities.",
			})
		} else {
			health, healthErr := base.Health(ctx, plugin, discovery.ID(plugin))
			if healthErr != nil {
				status, summary := "error", "Adapter health checks could not be completed."
				if strings.Contains(healthErr.Error(), "timeout") {
					status, summary = "warning", "Adapter health checks timed out."
				}
				findings = append(findings, Finding{
					Component: "plugin." + id + ".health", Status: status,
					Summary: summary,
					Repair:  "Check the adapter's local health mode and protocol v2 response.",
				})
			} else {
				for _, check := range health.Checks {
					findings = append(findings, Finding{
						Component: "plugin." + id + "." + sanitizeAdapterText(check.Code),
						Status:    check.Status,
						Summary:   sanitizeAdapterText(check.Summary),
						Repair:    sanitizeAdapterText(check.Repair),
					})
				}
			}
		}

		current, currentErr := base.Run(ctx, plugin, protocol.Request{
			Protocol: protocol.Version, ID: discovery.ID(plugin), Mode: protocol.ModeCurrent,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		if currentErr != nil {
			findings = append(findings, Finding{
				Component: "plugin." + id + ".current", Status: "warning",
				Summary: "The local current-version probe failed.",
				Repair:  "Check the adapter's current mode and local command installation.",
			})
		} else if current.Status == "unknown" || current.Status == "not_installed" {
			findings = append(findings, Finding{
				Component: "plugin." + id + ".current", Status: "warning",
				Summary: "The local probe could not confirm an installed version.",
				Repair:  "Check whether the tool is installed and available on PATH.",
			})
		} else {
			findings = append(findings, Finding{
				Component: "plugin." + id + ".current", Status: "ok",
				Summary: "The local current-version probe completed.",
			})
		}

		findings = append(findings, inspectTarget(store, id, discovery.ID(plugin))...)
		findings = append(findings, inspectCurrent(store, id, discovery.ID(plugin))...)
		findings = append(findings, inspectControl(store, id, discovery.ID(plugin))...)
		findings = append(findings, inspectRefresh(store, id, discovery.ID(plugin))...)
	}
	return findings
}

func inspectTarget(store cache.Store, displayID, cacheID string) []Finding {
	result, state := store.InspectTarget(cacheID)
	component := "cache." + displayID + ".target"
	switch state {
	case cache.RecordMissing:
		return []Finding{{Component: component, Status: "warning", Summary: "No target cache record exists.", Repair: "Run check-now refresh --force to request fresh target data."}}
	case cache.RecordCorrupt:
		return []Finding{{Component: component, Status: "error", Summary: "The target cache record is corrupt.", Repair: "Run check-now refresh --force to replace it after confirming the adapter works."}}
	case cache.RecordUnreadable:
		return []Finding{{Component: component, Status: "error", Summary: "The target cache record cannot be read.", Repair: "Check cache directory permissions."}}
	case cache.RecordLegacy:
		return []Finding{{Component: component, Status: "warning", Summary: "A legacy target record was found and left unchanged during diagnosis.", Repair: "Run check-now refresh --force to migrate it through the normal refresh path."}}
	case cache.RecordValid:
		age := recordAge(result.CheckedAt)
		if age < 0 || age > store.Stale {
			return []Finding{{Component: component, Status: "warning", Summary: staleTargetSummary(store, cacheID, age), Repair: "Check adapter configuration or connectivity, then run check-now refresh --force."}}
		}
		return []Finding{{Component: component, Status: "ok", Summary: "Target cache data is present and fresh."}}
	default:
		return []Finding{{Component: component, Status: "error", Summary: "Target cache state could not be determined."}}
	}
}

func staleTargetSummary(store cache.Store, id string, age time.Duration) string {
	summary := "Target cache data is stale."
	if age >= 0 {
		summary = fmt.Sprintf("Target cache data is stale (%s old).", age.Round(time.Minute))
	}
	if record, ok := store.ReadRefresh(id); ok && record.Outcome != "success" {
		summary += " Latest refresh outcome: " + record.Outcome + " at " + record.CheckedAt + "."
	}
	return summary
}

func inspectCurrent(store cache.Store, displayID, cacheID string) []Finding {
	result, state := store.InspectCurrent(cacheID)
	component := "cache." + displayID + ".current"
	switch state {
	case cache.RecordMissing:
		return []Finding{{Component: component, Status: "warning", Summary: "No current-version cache record exists."}}
	case cache.RecordCorrupt:
		return []Finding{{Component: component, Status: "error", Summary: "The current-version cache record is corrupt.", Repair: "Run a local shell check to replace current-version cache data."}}
	case cache.RecordUnreadable:
		return []Finding{{Component: component, Status: "error", Summary: "The current-version cache record cannot be read.", Repair: "Check cache directory permissions."}}
	case cache.RecordValid:
		age := recordAge(result.CheckedAt)
		if age < 0 || age > store.Stale {
			return []Finding{{Component: component, Status: "warning", Summary: "Current-version cache data is stale.", Repair: "Run check-now shell to refresh the local current-version probe."}}
		}
		return []Finding{{Component: component, Status: "ok", Summary: "Current-version cache data is present and fresh."}}
	default:
		return []Finding{{Component: component, Status: "error", Summary: "Current-version cache state could not be determined."}}
	}
}

func inspectControl(store cache.Store, displayID, cacheID string) []Finding {
	_, state := store.InspectControl(cacheID)
	component := "cache." + displayID + ".control"
	switch state {
	case cache.RecordMissing, cache.RecordValid:
		return []Finding{{Component: component, Status: "ok", Summary: "Control cache state is readable."}}
	case cache.RecordCorrupt:
		return []Finding{{Component: component, Status: "warning", Summary: "The control cache record is corrupt.", Repair: "Run check-now dismiss <plugin-id> after confirming target data is valid."}}
	case cache.RecordUnreadable:
		return []Finding{{Component: component, Status: "error", Summary: "The control cache record cannot be read.", Repair: "Check cache directory permissions."}}
	default:
		return []Finding{{Component: component, Status: "error", Summary: "Control cache state could not be determined."}}
	}
}

func inspectRefresh(store cache.Store, displayID, cacheID string) []Finding {
	record, state := store.InspectRefresh(cacheID)
	component := "cache." + displayID + ".refresh"
	switch state {
	case cache.RecordMissing:
		return []Finding{{Component: component, Status: "warning", Summary: "No refresh outcome has been recorded.", Repair: "Run check-now refresh --force to check the adapter and record an outcome."}}
	case cache.RecordCorrupt:
		return []Finding{{Component: component, Status: "warning", Summary: "Refresh history is unreadable or has an unsupported outcome.", Repair: "Run check-now refresh --force to record a new outcome."}}
	case cache.RecordUnreadable:
		return []Finding{{Component: component, Status: "error", Summary: "Refresh history cannot be read.", Repair: "Check cache directory permissions."}}
	case cache.RecordValid:
		if record.Outcome == "success" {
			return []Finding{{Component: component, Status: "ok", Summary: "The latest refresh completed successfully at " + record.CheckedAt + "."}}
		}
		return []Finding{{Component: component, Status: "warning", Summary: "The latest refresh outcome was " + record.Outcome + " at " + record.CheckedAt + ".", Repair: "Check the adapter's configuration or connectivity, then run check-now refresh --force."}}
	default:
		return []Finding{{Component: component, Status: "error", Summary: "Refresh history state could not be determined."}}
	}
}

func recordAge(checkedAt string) time.Duration {
	t, err := time.Parse(time.RFC3339, checkedAt)
	if err != nil {
		return -1
	}
	return time.Since(t)
}

func probeCache(dir string) Finding {
	component := "cache.writable"
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Finding{Component: component, Status: "warning", Summary: "The cache directory does not exist.", Repair: "Run check-now refresh to initialize the cache directory."}
		}
		return Finding{Component: component, Status: "error", Summary: "The cache directory cannot be inspected.", Repair: "Check cache directory permissions."}
	}
	if !info.IsDir() {
		return Finding{Component: component, Status: "error", Summary: "The cache location is not a directory.", Repair: "Choose a writable cache directory."}
	}
	file, err := os.CreateTemp(dir, ".doctor-probe-*")
	if err != nil {
		return Finding{Component: component, Status: "error", Summary: "The cache directory is not writable.", Repair: "Check cache directory permissions and available disk space."}
	}
	name := file.Name()
	probe := []byte("check-now doctor cache probe")
	_, writeErr := file.Write(probe)
	closeErr := file.Close()
	var readErr error
	if writeErr == nil && closeErr == nil {
		var got []byte
		got, readErr = os.ReadFile(name)
		if readErr == nil && !bytes.Equal(got, probe) {
			readErr = fmt.Errorf("probe content mismatch")
		}
	}
	removeErr := os.Remove(name)
	if writeErr != nil || closeErr != nil || readErr != nil || removeErr != nil {
		return Finding{Component: component, Status: "error", Summary: "The cache create, read, or cleanup probe failed.", Repair: "Check cache directory permissions and available disk space."}
	}
	return Finding{Component: component, Status: "ok", Summary: "The cache directory passed a temporary create, read, and remove probe."}
}

func WriteFindings(w io.Writer, findings []Finding) {
	for _, finding := range findings {
		fmt.Fprintf(w, "[%s] %s: %s\n", strings.ToUpper(finding.Status), finding.Component, safeText(finding.Summary))
		if finding.Repair != "" {
			fmt.Fprintf(w, "  Suggested repair: %s\n", safeText(finding.Repair))
		}
	}
}

func safeText(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
		if b.Len() >= 240 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func newAdapterTextSanitizer() func(string) string {
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, variable := range os.Environ() {
		_, envValue, ok := strings.Cut(variable, "=")
		if !ok || envValue == "" {
			continue
		}
		if _, ok := seen[envValue]; ok {
			continue
		}
		seen[envValue] = struct{}{}
		values = append(values, envValue)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return func(value string) string {
		for _, envValue := range values {
			value = strings.ReplaceAll(value, envValue, "[redacted]")
		}
		return safeText(value)
	}
}

func displayID(value string) string {
	value = filepath.Base(value)
	return safeText(value)
}
