package main

import (
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"check-now/internal/completion"
	"check-now/internal/scaffold"
)

type helpEntry struct {
	usage, description, arguments, example string
}

var helpCatalog = map[string]helpEntry{
	"version":       {"check-now version", "Print the installed release version.", "", "check-now version"},
	"shell":         {"check-now shell", "Show cached update information and safe commands.", "", "check-now shell"},
	"refresh":       {"check-now refresh [--force]", "Refresh plugin update information.", "  --force  Refresh immediately, even when the cache is current.", "check-now refresh --force"},
	"dismiss":       {"check-now dismiss <plugin-id>", "Hide displayed update targets for a plugin.", "  <plugin-id>  Canonical plugin ID.", "check-now dismiss node"},
	"list":          {"check-now list", "List discovered plugins and their paths.", "", "check-now list"},
	"doctor":        {"check-now doctor", "Report local adapter, configuration, and cache findings.", "", "check-now doctor"},
	"plugin create": {"check-now plugin create [<id>] [options]", "Create a local checker plugin from the interactive scaffold.", "", "check-now plugin create nodejs --name 'Node.js' --non-interactive"},
	"completion":    {"check-now completion [shell]", "Print a completion script for a supported shell.", "  [shell]  bash, zsh, fish, powershell, or pwsh; detected when omitted.", "check-now completion zsh"},
	"help":          {"check-now help [command]", "Show help for a command.", "  [command]  Command name, optionally including a subcommand.", "check-now help plugin create"},
}

func helpPage(args []string) (string, bool, error) {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "-h" || args[0] == "--help")) {
		return renderHelp("", false), true, nil
	}
	if args[0] == "help" {
		if len(args) == 1 || (len(args) == 2 && (args[1] == "-h" || args[1] == "--help")) {
			return renderHelp("", false), true, nil
		}
		commandArgs := args[1:]
		if commandArgs[len(commandArgs)-1] == "-h" || commandArgs[len(commandArgs)-1] == "--help" {
			commandArgs = commandArgs[:len(commandArgs)-1]
		}
		if len(commandArgs) == 0 {
			return renderHelp("", false), true, nil
		}
		key := strings.Join(commandArgs, " ")
		if _, ok := helpCatalog[key]; !ok {
			return "", true, fmt.Errorf("unknown command %q", key)
		}
		return renderHelp(key, true), true, nil
	}
	if args[len(args)-1] == "--help" || args[len(args)-1] == "-h" {
		key := strings.Join(args[:len(args)-1], " ")
		if key == "" {
			return renderHelp("", false), true, nil
		}
		if _, ok := helpCatalog[key]; !ok {
			return "", true, fmt.Errorf("unknown command %q", key)
		}
		return renderHelp(key, true), true, nil
	}
	return "", false, nil
}

func renderHelp(key string, detailed bool) string {
	if key == "plugin create" {
		return renderPluginCreateHelp()
	}
	if detailed {
		e := helpCatalog[key]
		var b strings.Builder
		fmt.Fprintf(&b, "check-now %s\n\n%s\n\nUsage\n  %s\n", key, e.description, e.usage)
		if e.arguments != "" {
			fmt.Fprintf(&b, "\nArguments and options\n%s\n", e.arguments)
		}
		fmt.Fprintf(&b, "\nExample\n  %s\n", e.example)
		if key == "completion" {
			fmt.Fprintf(&b, "\nSupported shells: %s\n", strings.Join(completion.SupportedShells(), ", "))
		}
		return b.String()
	}
	var b strings.Builder
	b.WriteString("check-now — cached developer-tool update checks\n\nUsage\n  check-now [command]\n\nCommands\n")
	keys := make([]string, 0, len(helpCatalog))
	for k := range helpCatalog {
		if k != "help" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "  %-18s %s\n", k, helpCatalog[k].description)
	}
	b.WriteString("  help               Show help for a command.\n\nGlobal options\n  -h, --help         Show help.\n\nRun 'check-now help <command>' for command details.\n")
	return b.String()
}

func renderPluginCreateHelp() string {
	var o scaffold.Options
	caps := ""
	nonInteractive := false
	f := newPluginCreateFlagSet(io.Discard, &o, &caps, &nonInteractive)
	var b strings.Builder
	b.WriteString("check-now plugin create\n\nCreate a local checker plugin from the interactive scaffold.\n\nUsage\n  check-now plugin create [<id>] [options]\n\nArguments and options\n  <id>  Optional plugin ID; prompts for required values when omitted.\n")
	f.VisitAll(func(v *flag.Flag) {
		fmt.Fprintf(&b, "  --%-16s %s\n", v.Name, v.Usage)
	})
	b.WriteString("\nExample\n  check-now plugin create nodejs --name 'Node.js' --non-interactive\n")
	return b.String()
}

func validateCommandArgs(args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "version", "shell", "list", "doctor":
		if len(args) != 1 {
			return fmt.Errorf("%s accepts no arguments", args[0])
		}
	case "refresh":
		if len(args) > 2 || (len(args) == 2 && args[1] != "--force" && args[1] != "--background") {
			return fmt.Errorf("refresh accepts only --force")
		}
	case "dismiss":
		if len(args) != 2 || strings.HasPrefix(args[1], "-") {
			return fmt.Errorf("dismiss requires one plugin ID")
		}
	case "completion":
		if len(args) > 2 {
			return fmt.Errorf("completion accepts at most one shell argument")
		}
		if len(args) == 2 {
			found := false
			for _, shell := range completion.SupportedShells() {
				found = found || args[1] == shell
			}
			if !found {
				return fmt.Errorf("unsupported completion shell %q", args[1])
			}
		}
	case "plugin":
		if len(args) < 2 || args[1] != "create" {
			return fmt.Errorf("expected 'plugin create'")
		}
		createArgs := args[2:]
		if len(createArgs) > 0 && !strings.HasPrefix(createArgs[0], "-") {
			createArgs = createArgs[1:]
		}
		var options scaffold.Options
		capabilities := ""
		nonInteractive := false
		flags := newPluginCreateFlagSet(io.Discard, &options, &capabilities, &nonInteractive)
		if err := flags.Parse(createArgs); err != nil {
			return fmt.Errorf("plugin create: %w", err)
		}
		if len(flags.Args()) > 1 {
			return fmt.Errorf("plugin create accepts at most one plugin ID")
		}
	case "help":
	case "-h", "--help":
		if len(args) != 1 {
			return fmt.Errorf("%s accepts no arguments", args[0])
		}
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	return nil
}

func newPluginCreateFlagSet(w io.Writer, o *scaffold.Options, caps *string, nonInteractive *bool) *flag.FlagSet {
	f := flag.NewFlagSet("plugin create", flag.ContinueOnError)
	f.SetOutput(w)
	f.StringVar(&o.ID, "id", "", "plugin ID (the folder name)")
	f.StringVar(&o.Name, "name", "", "display name")
	f.StringVar(&o.Description, "description", "", "one-sentence purpose")
	f.StringVar(&o.Tool, "tool", "", "optional local command used for detection")
	f.StringVar(&o.SourceURL, "source-url", "", "optional official source URL")
	f.StringVar(&o.Runtime, "runtime", "shell", "runtime (default: shell)")
	f.StringVar(&o.Manager, "manager", "", "optional installation manager")
	f.StringVar(caps, "capability", "current,refresh,health", "capabilities (default: current,refresh,health)")
	f.BoolVar(&o.Force, "force", false, "replace an existing plugin directory")
	f.BoolVar(nonInteractive, "non-interactive", false, "do not prompt")
	return f
}
