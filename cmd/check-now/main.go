package main

import (
	"bufio"
	"check-now/internal/app"
	"check-now/internal/discovery"
	"check-now/internal/runner"
	"check-now/internal/scaffold"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	exe, _ := os.Executable()
	builtin := os.Getenv("CHECK_NOW_BUILTIN_PLUGIN_DIR")
	var pluginDirs []string
	if builtin != "" {
		pluginDirs = append(pluginDirs, builtin)
	} else {
		pluginDirs = append(pluginDirs, discovery.BuiltinDirectories(exe)...)
	}
	plugins := discovery.Find("", append(app.ConfiguredPlugins(), pluginDirs...))
	valid := make([]string, 0, len(plugins))
	for _, p := range plugins {
		if _, e := (runner.Runner{Timeout: 5 * time.Second}).Metadata(context.Background(), p, discovery.ID(p)); e == nil {
			valid = append(valid, p)
		} else {
			fmt.Fprintf(os.Stderr, "check-now: ignoring %s: %v\n", discovery.ID(p), e)
		}
	}
	plugins = valid
	a := app.New(plugins, app.DefaultCache(), app.DurationEnv("CHECK_NOW_STALE_INTERVAL", 24*time.Hour), app.DurationEnv("CHECK_NOW_PLUGIN_TIMEOUT", 5*time.Second))
	var err error
	switch os.Args[1] {
	case "plugin":
		if len(os.Args) < 3 || os.Args[2] != "create" {
			usage()
			os.Exit(2)
		}
		err = createPlugin(os.Args[3:])
	case "shell":
		err = a.Shell(context.Background())
	case "refresh":
		err = a.Refresh(context.Background(), len(os.Args) > 2 && os.Args[2] == "--force")
	case "list":
		for _, p := range plugins {
			fmt.Printf("%s\t%s\n", discovery.ID(p), p)
		}
	case "doctor":
		if len(plugins) == 0 {
			fmt.Println("no plugins discovered")
			os.Exit(1)
		}
		fmt.Printf("plugins: %d\ncache: %s\n", len(plugins), app.DefaultCache())
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "check-now:", err)
		os.Exit(1)
	}
}
func createPlugin(args []string) error {
	f := flag.NewFlagSet("plugin create", flag.ContinueOnError)
	f.SetOutput(os.Stderr)
	var o scaffold.Options
	var caps string
	var nonInteractive bool
	positionalID := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalID, args = args[0], args[1:]
	}
	f.StringVar(&o.ID, "id", "", "plugin ID (the folder name)")
	f.StringVar(&o.Name, "name", "", "display name")
	f.StringVar(&o.Description, "description", "", "one-sentence purpose")
	f.StringVar(&o.Tool, "tool", "", "optional local command used for detection")
	f.StringVar(&o.SourceURL, "source-url", "", "optional official source URL")
	f.StringVar(&o.Runtime, "runtime", "shell", "runtime (default: shell)")
	f.StringVar(&o.Manager, "manager", "", "optional installation manager")
	f.StringVar(&caps, "capability", "refresh", "capability (default: refresh)")
	f.BoolVar(&o.Force, "force", false, "replace an existing plugin directory")
	f.BoolVar(&nonInteractive, "non-interactive", false, "do not prompt")
	if err := f.Parse(args); err != nil {
		return err
	}
	if o.ID == "" && positionalID != "" {
		o.ID = positionalID
	} else if o.ID == "" && len(f.Args()) > 0 {
		o.ID = f.Args()[0]
	}
	if !nonInteractive {
		promptPlugin(&o, &caps, bufio.NewReader(os.Stdin))
	}
	if caps == "" {
		o.Capabilities = []string{"refresh"}
	} else {
		o.Capabilities = splitCaps(caps)
	}
	o.Root = pluginRoot()
	dir, err := scaffold.Create(o.Root, o)
	if err != nil {
		return err
	}
	fmt.Printf("Created plugin %s\n", dir)
	return nil
}

func promptPlugin(o *scaffold.Options, caps *string, r *bufio.Reader) {
	o.ID = ask(r, "Plugin ID", "required; becomes the folder name", o.ID, true)
	o.Name = ask(r, "Display name", "required; shown to users", o.Name, true)
	o.Description = ask(r, "Description", "required; one-sentence purpose", o.Description, true)
	o.Tool = ask(r, "Command/tool", "optional; local command used for detection", o.Tool, false)
	o.SourceURL = ask(r, "Official source URL", "optional; release/download/documentation URL", o.SourceURL, false)
	o.Runtime = ask(r, "Runtime", "defaults to shell", o.Runtime, false)
	o.Manager = ask(r, "Installation manager", "optional; for example fnm, Homebrew, apt, npm, or asdf", o.Manager, false)
	*caps = ask(r, "Capabilities", "defaults to refresh", *caps, false)
}

func ask(r *bufio.Reader, label, help, value string, required bool) string {
	optional := ""
	if !required {
		optional = " (optional)"
	}
	fmt.Printf("%s%s — %s", label, optional, help)
	if value != "" {
		fmt.Printf(" [%s]", value)
	}
	fmt.Print(": ")
	s, _ := r.ReadString('\n')
	s = strings.TrimSpace(s)
	if s == "" {
		return value
	}
	return s
}

func splitCaps(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func pluginRoot() string {
	if v := os.Getenv("CHECK_NOW_PLUGIN_ROOT"); v != "" {
		return v
	}
	if v := os.Getenv("CHECK_NOW_PLUGIN_DIRS"); v != "" {
		return strings.Split(v, string(os.PathListSeparator))[0]
	}
	return filepath.Join("plugins")
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: check-now {shell|refresh [--force]|list|doctor|plugin create <id> [flags]}")
}
