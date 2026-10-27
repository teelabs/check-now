package app

import (
	"check-now/internal/cache"
	"check-now/internal/discovery"
	"check-now/internal/protocol"
	"check-now/internal/runner"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type App struct {
	Plugins []string
	Store   cache.Store
	Timeout time.Duration
}

func New(plugins []string, cacheDir string, stale, timeout time.Duration) App {
	return App{plugins, cache.New(cacheDir, stale), timeout}
}
func (a App) Refresh(ctx context.Context, force bool) error {
	unlock, e := a.Store.Lock("refresh", a.Timeout+time.Second)
	if e != nil {
		return e
	}
	defer unlock()
	var failures []error
	for _, p := range a.Plugins {
		id := discovery.ID(p)
		old, _ := a.Store.ReadAny(id)
		if !force {
			if _, ok := a.Store.Read(id); ok {
				continue
			}
		}
		req := protocol.Request{Protocol: protocol.Version, ID: id, Mode: "refresh", Timestamp: time.Now().UTC().Format(time.RFC3339), Prior: &old}
		r, e := runner.Runner{Timeout: a.Timeout}.Run(ctx, p, req)
		if e != nil {
			fmt.Fprintf(os.Stderr, "check-now: %s: %v\n", id, e)
			failures = append(failures, e)
			continue
		}
		if r.Status == "unknown" && old.ID == id {
			fmt.Fprintf(os.Stderr, "check-now: %s: preserving previous cache after unknown refresh\n", id)
			continue
		}
		if e := a.Store.Write(r); e != nil {
			return e
		}
	}
	if len(failures) > 0 {
		return errors.New("one or more plugins failed to refresh")
	}
	return nil
}
func (a App) Shell(ctx context.Context) error {
	stale := false
	var visible []protocol.Result
	for _, p := range a.Plugins {
		id := discovery.ID(p)
		r, ok := a.Store.ReadAny(id)
		if !ok {
			stale = true
			continue
		}
		t, _ := time.Parse(time.RFC3339, r.CheckedAt)
		if time.Since(t) > a.Store.Stale {
			stale = true
		}
		if r.Status == "update_available" || r.Status == "unknown" {
			visible = append(visible, r)
		}
	}
	if len(visible) > 0 {
		fmt.Print(formatUpdates(visible))
	}
	if stale {
		if exe, err := os.Executable(); err == nil {
			cmd := exec.Command(exe, "refresh")
			cmd.Stdout = nil
			cmd.Stderr = nil
			_ = cmd.Start()
		}
	}
	return nil
}

type updateRow struct {
	name, version, command, source string
}

func formatUpdates(results []protocol.Result) string {
	var available, attention []updateRow
	for _, r := range results {
		updates := r.Updates
		if len(updates) == 0 && r.TargetVersion != "" {
			updates = []protocol.Update{{Scope: "latest", Track: r.Track, Version: r.TargetVersion, SourceURL: r.SourceURL, Argv: r.Argv}}
		}
		if len(updates) == 0 {
			row := updateRow{r.Tool, r.CurrentVersion, "Not available", r.SourceURL}
			if row.version == "" {
				row.version = "Unknown"
			}
			attention = append(attention, row)
			continue
		}
		for _, u := range updates {
			version := u.Version
			if u.Track != "" {
				version += " (" + u.Track + ")"
			}
			command := "Not available"
			if len(u.Argv) > 0 {
				command = protocol.QuoteArgv(u.Argv)
			}
			row := updateRow{r.Tool, version, command, u.SourceURL}
			if row.source == "" {
				row.source = r.SourceURL
			}
			if r.Status == "update_available" {
				available = append(available, row)
			} else {
				attention = append(attention, row)
			}
		}
	}
	var b strings.Builder
	if len(available) > 0 {
		writeUpdateTable(&b, "Updates available!", available)
	}
	if len(attention) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		writeUpdateTable(&b, "Some updates could not be verified.", attention)
	}
	return b.String()
}

func writeUpdateTable(b *strings.Builder, title string, rows []updateRow) {
	headers := []string{"Name", "Version", "Command to update", "Source"}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	values := make([][]string, len(rows))
	for i, r := range rows {
		value := []string{tableCell(r.name), tableCell(r.version), tableCell(r.command), tableCell(r.source)}
		values[i] = value
	}
	displayValues := expandGroupedRows(values)
	for i := range values {
		for j, value := range values[i] {
			if len(value) > widths[j] {
				widths[j] = len(value)
			}
		}
	}
	middle := tableLine(widths, "├", "┼", "┤")
	bottom := tableLine(widths, "└", "┴", "┘")
	titleWidth := tableInnerWidth(widths)
	top := "┌" + strings.Repeat("─", titleWidth+2) + "┐"
	titleBottom := tableLine(widths, "├", "┬", "┤")
	fmt.Fprintf(b, "%s\n", top)
	fmt.Fprintf(b, "│ %s │\n", centerText(title, titleWidth))
	fmt.Fprintf(b, "%s\n", titleBottom)
	writeTableRow(b, headers, widths, []bool{true, true, true, true})
	fmt.Fprintf(b, "%s\n", middle)
	for _, value := range displayValues {
		writeTableRow(b, value, widths, []bool{true, true, false, true})
	}
	fmt.Fprintf(b, "%s\n", bottom)
}

func tableLine(widths []int, left, junction, right string) string {
	var b strings.Builder
	b.WriteString(left)
	for i, width := range widths {
		b.WriteString(strings.Repeat("─", width+2))
		if i == len(widths)-1 {
			b.WriteString(right)
		} else {
			b.WriteString(junction)
		}
	}
	return b.String()
}

func tableInnerWidth(widths []int) int {
	width := len(widths) - 1
	for _, column := range widths {
		width += column + 2
	}
	return width - 2
}

func writeTableRow(b *strings.Builder, values []string, widths []int, centered []bool) {
	b.WriteString("│")
	for i, value := range values {
		if centered[i] {
			value = centerText(value, widths[i])
		} else {
			value = fmt.Sprintf("%-*s", widths[i], value)
		}
		fmt.Fprintf(b, " %s │", value)
	}
	b.WriteByte('\n')
}

func expandGroupedRows(values [][]string) [][]string {
	var out [][]string
	for start := 0; start < len(values); {
		end := start + 1
		for end < len(values) && values[end][0] == values[start][0] && values[end][3] == values[start][3] {
			end++
		}
		group := values[start:end]
		if len(group) == 1 {
			out = append(out, group[0])
		} else {
			displayStart := len(out)
			for i, row := range group {
				if i > 0 {
					out = append(out, []string{"", "", "", ""})
				}
				copyRow := append([]string(nil), row...)
				copyRow[0], copyRow[3] = "", ""
				out = append(out, copyRow)
			}
			center := displayStart + len(group) - 1
			out[center][0], out[center][3] = group[0][0], group[0][3]
		}
		start = end
	}
	return out
}

func centerText(value string, width int) string {
	if len(value) >= width {
		return value
	}
	left := (width - len(value)) / 2
	right := width - len(value) - left
	return strings.Repeat(" ", left) + value + strings.Repeat(" ", right)
}

func tableCell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ")
}

func DefaultCache() string {
	if x := os.Getenv("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "check-now")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".cache", "check-now")
}
func DurationEnv(name string, def time.Duration) time.Duration {
	if s := os.Getenv(name); s != "" {
		if d, e := time.ParseDuration(s); e == nil {
			return d
		}
		if n, e := strconv.Atoi(s); e == nil {
			return time.Duration(n) * time.Hour
		}
	}
	return def
}
func ConfiguredPlugins() []string {
	var out []string
	for _, d := range strings.Split(os.Getenv("CHECK_NOW_PLUGIN_DIRS"), string(os.PathListSeparator)) {
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}
