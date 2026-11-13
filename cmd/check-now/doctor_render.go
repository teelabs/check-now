package main

import (
	"check-now/internal/app"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	doctorANSIReset     = "\x1b[0m"
	doctorANSIHeader    = "\x1b[38;2;88;166;255m"
	doctorANSICommand   = "\x1b[38;2;63;185;80m"
	doctorANSILocation  = "\x1b[38;2;121;192;255m"
	doctorANSIWarning   = "\x1b[38;2;210;153;34m"
	doctorANSIFailure   = "\x1b[38;2;248;81;73m"
	doctorANSIIdentity  = "\x1b[38;2;210;168;255m"
	doctorANSISecondary = "\x1b[38;2;139;148;158m"
)

type doctorRenderer struct {
	w     io.Writer
	color bool
}

func renderDoctor(w io.Writer, findings []app.Finding, color bool) {
	renderer := doctorRenderer{w: w, color: color}
	renderer.header("check-now doctor")

	configuration := make([]app.Finding, 0)
	adapters := make([]app.Finding, 0)
	cacheFindings := make([]app.Finding, 0)
	for _, finding := range findings {
		switch {
		case finding.Plugin != "" && strings.HasPrefix(finding.Component, "plugin."):
			adapters = append(adapters, finding)
		case strings.HasPrefix(finding.Component, "cache."):
			cacheFindings = append(cacheFindings, finding)
		default:
			configuration = append(configuration, finding)
		}
	}

	renderer.section("Configuration")
	settings := make([]app.Finding, 0)
	pluginRoots := make([]app.Finding, 0)
	for _, finding := range configuration {
		if strings.HasPrefix(finding.Component, "settings.") {
			settings = append(settings, finding)
		} else {
			pluginRoots = append(pluginRoots, finding)
		}
	}
	renderer.subsection("Settings")
	for _, finding := range settings {
		renderer.finding(finding, strings.TrimPrefix(finding.Component, "settings."), "    ")
	}
	if len(pluginRoots) > 0 {
		renderer.subsection("Plugin roots")
		for _, finding := range pluginRoots {
			renderer.finding(finding, strings.TrimPrefix(finding.Component, "plugins."), "    ")
		}
	}

	renderer.section("Adapters")
	adapterGroups := groupByPlugin(adapters)
	if len(adapterGroups) == 0 {
		renderer.text(doctorANSISecondary, "  No adapters discovered.")
	}
	for _, group := range adapterGroups {
		renderer.text(doctorANSIIdentity, "  "+group.plugin)
		for _, finding := range group.findings {
			label := strings.TrimPrefix(finding.Component, "plugin."+finding.Plugin+".")
			renderer.finding(finding, label, "    ")
		}
	}

	renderer.section("Cache")
	cacheGroups := make([]app.Finding, 0)
	for _, finding := range cacheFindings {
		if finding.Plugin == "" {
			label := strings.TrimPrefix(finding.Component, "cache.")
			renderer.finding(finding, label, "  ")
		} else {
			cacheGroups = append(cacheGroups, finding)
		}
	}
	for _, group := range groupByPlugin(cacheGroups) {
		renderer.text(doctorANSIIdentity, "  "+group.plugin)
		for _, finding := range group.findings {
			label := strings.TrimPrefix(finding.Component, "cache."+finding.Plugin+".")
			renderer.finding(finding, label, "    ")
		}
	}
}

type doctorPluginGroup struct {
	plugin   string
	findings []app.Finding
}

func groupByPlugin(findings []app.Finding) []doctorPluginGroup {
	groups := make([]doctorPluginGroup, 0)
	indexes := make(map[string]int)
	for _, finding := range findings {
		index, exists := indexes[finding.Plugin]
		if !exists {
			index = len(groups)
			indexes[finding.Plugin] = index
			groups = append(groups, doctorPluginGroup{plugin: finding.Plugin})
		}
		groups[index].findings = append(groups[index].findings, finding)
	}
	return groups
}

func (r doctorRenderer) header(title string) {
	r.text(doctorANSIHeader, "\n"+title)
	r.text(doctorANSIHeader, strings.Repeat("=", len(title)))
}

func (r doctorRenderer) section(title string) {
	r.text(doctorANSIHeader, "\n"+title)
	r.text(doctorANSIHeader, strings.Repeat("─", len(title)))
}

func (r doctorRenderer) subsection(title string) {
	r.text(doctorANSISecondary, "  "+title)
}

func (r doctorRenderer) finding(finding app.Finding, label, indent string) {
	status := strings.ToUpper(strings.ReplaceAll(finding.Status, "_", " "))
	statusColor := doctorANSISecondary
	switch finding.Status {
	case "ok":
		statusColor = doctorANSICommand
	case "warning":
		statusColor = doctorANSIWarning
	case "error":
		statusColor = doctorANSIFailure
	}
	fmt.Fprintf(r.w, "%s%s %s: %s\n", indent, r.colorize(statusColor, "["+status+"]"), r.colorize(doctorANSILocation, label), finding.Summary)
	if finding.Repair != "" {
		repairColor := doctorANSISecondary
		if strings.Contains(finding.Repair, "check-now ") {
			repairColor = doctorANSICommand
		}
		fmt.Fprintf(r.w, "%s  %s %s\n", indent, r.colorize(doctorANSISecondary, "Suggested repair:"), r.colorize(repairColor, finding.Repair))
	}
}

func (r doctorRenderer) text(color, text string) {
	fmt.Fprintln(r.w, r.colorize(color, text))
}

func (r doctorRenderer) colorize(color, value string) string {
	if !r.color || value == "" {
		return value
	}
	return color + value + doctorANSIReset
}

func doctorColorEnabled(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
