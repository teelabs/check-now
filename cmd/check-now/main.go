package main

import (
	"bufio"
	"check-now/internal/app"
	"check-now/internal/completion"
	"check-now/internal/discovery"
	"check-now/internal/runner"
	"check-now/internal/scaffold"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var version = "dev"

func main() {
	if page, handled, err := helpPage(os.Args[1:]); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, "check-now:", err)
			fmt.Fprintln(os.Stderr, "Run 'check-now help'")
			os.Exit(2)
		}
		fmt.Fprint(os.Stdout, page)
		return
	}
	if err := validateCommandArgs(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "check-now:", err)
		fmt.Fprintln(os.Stderr, "Run 'check-now help'")
		os.Exit(2)
	}
	if len(os.Args) < 2 {
		return
	}
	if os.Args[1] == "completion" {
		if err := completionCommand(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "check-now:", err)
			os.Exit(2)
		}
		return
	}
	if os.Args[1] == "version" {
		if len(os.Args) != 2 {
			usage()
			os.Exit(2)
		}
		fmt.Printf("check-now %s\n", version)
		return
	}
	if os.Args[1] == "doctor" {
		doctorCommand()
		return
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
		background := len(os.Args) > 2 && os.Args[2] == "--background"
		if background {
			perPlugin := app.DurationEnv("CHECK_NOW_PLUGIN_TIMEOUT", 5*time.Second)
			if perPlugin <= 0 {
				perPlugin = 5 * time.Second
			}
			deadline := perPlugin * time.Duration(len(plugins)+1)
			ctx, cancel := context.WithTimeout(context.Background(), deadline)
			defer cancel()
			err = a.RefreshBackground(ctx)
		} else {
			err = a.Refresh(context.Background(), len(os.Args) > 2 && os.Args[2] == "--force")
		}
	case "dismiss":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		err = a.Dismiss(os.Args[2])
	case "list":
		for _, p := range plugins {
			fmt.Printf("%s\t%s\n", discovery.ID(p), p)
		}
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
	var o scaffold.Options
	caps := "current,refresh,health"
	nonInteractive := false
	f := newPluginCreateFlagSet(os.Stderr, &o, &caps, &nonInteractive)
	positionalID := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalID, args = args[0], args[1:]
	}
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
		o.Capabilities = []string{"current", "refresh", "health"}
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
	*caps = ask(r, "Capabilities", "defaults to current, refresh, and health", *caps, false)
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
	fmt.Fprintln(os.Stderr, "Run 'check-now help'")
}

func completionCommand(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("completion accepts at most one shell argument (supported: %s)", strings.Join(completion.SupportedShells(), ", "))
	}
	shell := ""
	if len(args) == 1 {
		shell = args[0]
	} else {
		env := make(map[string]string)
		for _, entry := range os.Environ() {
			key, value, ok := strings.Cut(entry, "=")
			if ok {
				env[key] = value
			}
		}
		var err error
		shell, err = completion.DetectShell(env)
		if err != nil {
			return err
		}
	}
	script, err := completion.Script(shell)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(os.Stdout, script)
	return err
}
