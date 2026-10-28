// PageVault is the browser's native host, local MCP server and per-user installer.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/akarazhev/pagevault/internal/buildinfo"
	"github.com/akarazhev/pagevault/internal/install"
	"github.com/akarazhev/pagevault/internal/mcpserver"
	"github.com/akarazhev/pagevault/internal/native"
)

func main() {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "PageVault:", err)
		os.Exit(1)
	}
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, executable))
}

func run(args []string, input io.Reader, output, errorOutput io.Writer, executable string) int {
	command := "serve"
	if len(args) > 0 {
		switch args[0] {
		case "serve", "mcp", "install", "uninstall", "diagnose", "version", "package-preflight":
			command, args = args[0], args[1:]
		case "--version":
			command, args = "version", args[1:]
		case "help", "--help", "-h":
			fmt.Fprint(output, usage)
			return 0
		}
	}
	var err error
	switch command {
	case "package-preflight":
		flags := flag.NewFlagSet(command, flag.ContinueOnError)
		flags.SetOutput(errorOutput)
		home := flags.String("home", "", "Absolute user home")
		err = flags.Parse(args)
		if err == nil && flags.NArg() != 0 {
			err = fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
		}
		if err == nil {
			err = install.CheckPackagePayload(*home)
		}
	case "mcp":
		flags := flag.NewFlagSet(command, flag.ContinueOnError)
		flags.SetOutput(errorOutput)
		path := flags.String("db", "", "Existing SQLite file (read-only)")
		err = flags.Parse(args)
		if err == nil && flags.NArg() != 0 {
			err = fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
		}
		if err == nil {
			*path, err = databasePath(*path, executable)
		}
		if err == nil {
			err = mcpserver.Serve(context.Background(), *path, input, output)
		}
	case "version":
		fmt.Fprintf(output, "PageVault %s (%s, %s/%s)\n", buildinfo.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return 0
	case "install", "uninstall":
		options := install.Options{ExecutablePath: executable}
		flags := flag.NewFlagSet(command, flag.ContinueOnError)
		flags.SetOutput(errorOutput)
		flags.StringVar(&options.Browser, "browser", "chrome", "chrome, chromium, edge or brave")
		flags.StringVar(&options.ExtensionID, "extension-id", "", "Extension ID (defaults to the standard PageVault ID)")
		flags.StringVar(&options.ExtensionManifest, "extension-manifest", "", "manifest.json used to derive the extension ID")
		flags.StringVar(&options.DBPath, "db", "", "SQLite file")
		flags.StringVar(&options.RuntimeDir, "runtime-dir", "", "Installed host directory")
		flags.StringVar(&options.ManifestDir, "manifest-dir", "", "NativeMessagingHosts directory")
		flags.StringVar(&options.Home, "home", "", "Home directory for testing or preview")
		flags.StringVar(&options.Platform, "platform", "", "Linux, Darwin or Windows; another OS requires --dry-run")
		flags.BoolVar(&options.DryRun, "dry-run", false, "Show the plan without changing files")
		flags.BoolVar(&options.PreserveRegisteredDB, "preserve-registered-db", false, "Preserve the registered native host database path")
		flags.BoolVar(&options.Uninstall, "uninstall", command == "uninstall", "Remove registration while preserving the database")
		err = flags.Parse(args)
		if err == nil && flags.NArg() != 0 {
			err = fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
		}
		if err == nil {
			var plan map[string]any
			plan, err = install.Run(options)
			if err == nil {
				err = printJSON(output, plan)
			}
		}
	default:
		var path string
		var diagnose bool
		path, diagnose, err = parseServeArgs(args, errorOutput)
		if err == nil {
			path, err = databasePath(path, executable)
		}
		if err == nil {
			if command == "diagnose" || diagnose {
				var result map[string]any
				result, err = native.Diagnose(path)
				if err == nil {
					result["ok"] = true
					err = printJSON(output, result)
				}
			} else {
				err = native.Serve(path, input, output)
			}
		}
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(errorOutput, "PageVault:", err)
		return 1
	}
	return 0
}

func parseServeArgs(args []string, errorOutput io.Writer) (string, bool, error) {
	// Chromium appends its extension origin and, on Windows, a parent-window value.
	// They are transport metadata, never a source of file paths or commands.
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "chrome-extension://") || strings.HasPrefix(arg, "--parent-window=") {
			continue
		}
		if arg == "--parent-window" && i+1 < len(args) {
			i++
			continue
		}
		filtered = append(filtered, arg)
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	path := flags.String("db", "", "SQLite file")
	diagnose := flags.Bool("diagnose", false, "Print diagnostic JSON instead of Native Messaging")
	if err := flags.Parse(filtered); err != nil {
		return "", false, err
	}
	if flags.NArg() != 0 {
		return "", false, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	return *path, *diagnose, nil
}

func databasePath(explicit, executable string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if value := os.Getenv("PAGEVAULT_DB"); value != "" {
		return value, nil
	}
	filename := filepath.Join(filepath.Dir(executable), "pagevault.json")
	raw, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var config struct {
		SchemaVersion int    `json:"schema_version"`
		DBPath        string `json:"db_path"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", fmt.Errorf("read %s: %w", filename, err)
	}
	if config.SchemaVersion != 1 || config.DBPath == "" || !filepath.IsAbs(config.DBPath) {
		return "", fmt.Errorf("invalid configuration %s; reinstall with --db", filename)
	}
	return config.DBPath, nil
}

func printJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

const usage = `PageVault — local web page archive

pagevault-host install --browser chrome [--db PATH] [--dry-run]
pagevault-host uninstall --browser chrome
pagevault-host diagnose [--db PATH]
pagevault-host version
pagevault-host package-preflight --home ABSOLUTE_HOME
pagevault-host serve [--db PATH]
pagevault-host mcp [--db PATH]

With no command, Native Messaging uses framed JSON on stdin/stdout.
The mcp command serves an existing archive read-only over stdio.
Browsers: chrome, chromium, edge, brave. See README.md.
`
