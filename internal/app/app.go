package app

import (
	"check-now/internal/cache"
	"check-now/internal/discovery"
	"check-now/internal/protocol"
	"check-now/internal/runner"
	"check-now/internal/semver"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type App struct {
	Plugins []string
	Store   cache.Store
	Timeout time.Duration
}

var startBackgroundRefresh = func(executable string) error {
	cmd := exec.Command(executable, "refresh", "--background")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

func New(plugins []string, cacheDir string, stale, timeout time.Duration) App {
	return App{plugins, cache.New(cacheDir, stale), timeout}
}
func (a App) Refresh(ctx context.Context, force bool) error {
	return a.refresh(ctx, force, false)
}

func (a App) RefreshBackground(ctx context.Context) error {
	return a.refresh(ctx, false, true)
}

func (a App) refresh(ctx context.Context, force, background bool) error {
	wait := a.Timeout + time.Second
	if background {
		wait = 0
	}
	unlock, e := a.Store.Lock("refresh", wait)
	if e != nil {
		return e
	}
	defer unlock()
	var failures []error
	for _, p := range a.Plugins {
		id := discovery.ID(p)
		old, _ := a.Store.ReadTarget(id)
		if force {
			c := a.Store.ReadControl(id)
			c.Dismissed = false
			if err := a.Store.WriteControl(id, c); err != nil {
				return err
			}
		}
		if !force {
			if _, ok := a.Store.ReadTargetFresh(id); ok {
				continue
			}
		}
		var prior *protocol.Result
		if old.ID == id {
			prior = &old
		}
		req := protocol.Request{Protocol: protocol.Version, ID: id, Mode: protocol.ModeRefresh, Timestamp: time.Now().UTC().Format(time.RFC3339), Prior: prior}
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
		previous := a.Store.ReadControl(id)
		if old.ID != id || cache.Fingerprint(old) != cache.Fingerprint(r) {
			previous.Dismissed = false
		}
		previous.Fingerprint = cache.Fingerprint(r)
		if e := a.Store.WriteTarget(r); e != nil {
			return e
		}
		if e := a.Store.WriteControl(id, previous); e != nil {
			return e
		}
	}
	if len(failures) > 0 {
		return errors.New("one or more plugins failed to refresh")
	}
	return nil
}
func (a App) Shell(ctx context.Context) error {
	type shellResult struct {
		index        int
		visible      *protocol.Result
		needsRefresh bool
	}
	results := make(chan shellResult, len(a.Plugins))
	var wg sync.WaitGroup
	for i, p := range a.Plugins {
		wg.Add(1)
		go func(index int, plugin string) {
			defer wg.Done()
			id := discovery.ID(plugin)
			current, currentErr := runner.Runner{Timeout: a.Timeout}.Run(ctx, plugin, protocol.Request{Protocol: protocol.Version, ID: id, Mode: protocol.ModeCurrent, Timestamp: time.Now().UTC().Format(time.RFC3339)})
			if currentErr == nil {
				_ = a.Store.WriteCurrent(current)
			}
			target, ok := a.Store.ReadTarget(id)
			if !ok {
				results <- shellResult{index: index, needsRefresh: true}
				return
			}
			t, targetErr := time.Parse(time.RFC3339, target.CheckedAt)
			stale := targetErr != nil || time.Since(t) > a.Store.Stale
			if currentErr != nil || targetErr != nil || current.Status == "not_installed" {
				results <- shellResult{index: index, needsRefresh: stale}
				return
			}
			combined := combineCurrent(target, current)
			if combined.Status != "update_available" && combined.Status != "unknown" {
				results <- shellResult{index: index, needsRefresh: stale}
				return
			}
			control := a.Store.ReadControl(id)
			if control.Dismissed && control.Fingerprint == cache.Fingerprint(target) {
				results <- shellResult{index: index, needsRefresh: stale}
				return
			}
			results <- shellResult{index: index, visible: &combined, needsRefresh: stale}
		}(i, p)
	}
	go func() { wg.Wait(); close(results) }()
	visibleByIndex := make([]*protocol.Result, len(a.Plugins))
	needsRefresh := false
	for result := range results {
		if result.visible != nil {
			visibleByIndex[result.index] = result.visible
		}
		needsRefresh = needsRefresh || result.needsRefresh
	}
	var visible []protocol.Result
	for _, result := range visibleByIndex {
		if result != nil {
			visible = append(visible, *result)
		}
	}
	if len(visible) > 0 {
		fmt.Print(formatUpdates(visible))
	}
	if needsRefresh && len(a.Plugins) > 0 {
		if exe, err := os.Executable(); err == nil {
			_ = startBackgroundRefresh(exe)
		}
	}
	return nil
}

func combineCurrent(target, current protocol.Result) protocol.Result {
	r := target
	r.CurrentVersion = current.CurrentVersion
	if current.Manager != "" {
		r.Manager = current.Manager
	}
	var updates []protocol.Update
	for _, u := range target.Updates {
		if current.CurrentVersion == "" {
			updates = append(updates, u)
			continue
		}
		newer, err := semver.Latest(current.CurrentVersion, u.Version)
		if err == nil && newer {
			updates = append(updates, u)
		}
	}
	if len(updates) == 0 && target.TargetVersion != "" {
		if current.CurrentVersion == "" {
			updates = append(updates, protocol.Update{Scope: "latest", Track: target.Track, Version: target.TargetVersion, SourceURL: target.SourceURL, Argv: target.Argv})
		} else if newer, err := semver.Latest(current.CurrentVersion, target.TargetVersion); err == nil && newer {
			updates = append(updates, protocol.Update{Scope: "latest", Track: target.Track, Version: target.TargetVersion, SourceURL: target.SourceURL, Argv: target.Argv})
		}
	}
	r.Updates = updates
	if len(updates) > 0 {
		if target.Status == "update_available" {
			r.Status = "update_available"
		} else {
			r.Status = "unknown"
		}
	} else if target.Status == "unknown" && target.TargetVersion == "" {
		r.Status = "unknown"
	} else {
		r.Status = "up_to_date"
	}
	return r
}

func (a App) Dismiss(id string) error {
	unlock, err := a.Store.Lock("refresh", a.Timeout+time.Second)
	if err != nil {
		return err
	}
	defer unlock()
	for _, p := range a.Plugins {
		if discovery.ID(p) != id {
			continue
		}
		target, ok := a.Store.ReadTarget(id)
		if !ok {
			return fmt.Errorf("no target cache for plugin %q", id)
		}
		return a.Store.WriteControl(id, cache.Control{Dismissed: true, Fingerprint: cache.Fingerprint(target)})
	}
	return fmt.Errorf("unknown plugin %q", id)
}

type updateRow struct {
	name, current, target, command, source string
}

func formatUpdates(results []protocol.Result) string {
	var available, attention []updateRow
	for _, r := range results {
		updates := r.Updates
		if len(updates) == 0 && r.TargetVersion != "" {
			updates = []protocol.Update{{Scope: "latest", Track: r.Track, Version: r.TargetVersion, SourceURL: r.SourceURL, Argv: r.Argv}}
		}
		if len(updates) == 0 {
			row := updateRow{r.Tool, r.CurrentVersion, "Not available", "Not available", r.SourceURL}
			if row.current == "" {
				row.current = "Unknown"
			}
			attention = append(attention, row)
			continue
		}
		for _, u := range updates {
			target := u.Version
			if u.Track != "" {
				target += " (" + u.Track + ")"
			}
			command := "Not available"
			if len(u.Argv) > 0 {
				command = protocol.QuoteArgv(u.Argv)
			}
			current := r.CurrentVersion
			if current == "" {
				current = "Unknown"
			}
			row := updateRow{r.Tool, current, target, command, u.SourceURL}
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
	headers := []string{"Name", "Current", "Target", "Command to update", "Source"}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	values := make([][]string, len(rows))
	for i, r := range rows {
		value := []string{tableCell(r.name), tableCell(r.current), tableCell(r.target), tableCell(r.command), tableCell(r.source)}
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
	writeTableRow(b, headers, widths, []bool{true, true, true, true, true})
	fmt.Fprintf(b, "%s\n", middle)
	for _, value := range displayValues {
		writeTableRow(b, value, widths, []bool{true, true, false, false, true})
	}
	fmt.Fprintf(b, "%s\n", bottom)
	fmt.Fprintf(b, "Hint: use check-now dismiss <plugin-id> to hide this notification.\n")
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
		for end < len(values) && values[end][0] == values[start][0] && values[end][4] == values[start][4] {
			end++
		}
		group := values[start:end]
		if len(group) == 1 {
			out = append(out, group[0])
		} else {
			displayStart := len(out)
			for i, row := range group {
				if i > 0 {
					out = append(out, []string{"", "", "", "", ""})
				}
				copyRow := append([]string(nil), row...)
				copyRow[0], copyRow[1], copyRow[4] = "", "", ""
				out = append(out, copyRow)
			}
			center := displayStart + len(group) - 1
			out[center][0], out[center][1], out[center][4] = group[0][0], group[0][1], group[0][4]
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
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	return strings.ReplaceAll(s, "|", "\\|")
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
