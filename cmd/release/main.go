// Command release builds and packages a PageVault release using only Go tools.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/akarazhev/pagevault/internal/buildinfo"
	"github.com/akarazhev/pagevault/internal/release"
)

func main() {
	var opts release.Options
	flag.StringVar(&opts.Output, "output", "", "output release ZIP")
	flag.StringVar(&opts.Go, "go", "go", "Go compiler executable")
	flag.BoolVar(&opts.SourceOnly, "source-only", false, "package source without compiling or including hosts")
	flag.BoolVar(&opts.SkipBuild, "skip-build", false, "package the six existing bin platform hosts without compiling")
	flag.BoolVar(&opts.ExtensionOnly, "extension-only", false, "package only the browser extension and LICENSE, with manifest.json at ZIP root")
	flag.StringVar(&opts.Browser, "browser", "chromium", "extension package browser: chromium or firefox")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "release: unexpected positional arguments")
		os.Exit(2)
	}
	if opts.Output == "" {
		if opts.ExtensionOnly {
			opts.Output = "dist/PageVault-Extension-" + buildinfo.Version + ".zip"
			if opts.Browser == "firefox" {
				opts.Output = "dist/PageVault-Firefox-" + buildinfo.Version + ".zip"
			}
		} else {
			opts.Output = "dist/PageVault-" + buildinfo.Version + ".zip"
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := release.Run(ctx, opts); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
	fmt.Printf("Created %s and %s.sha256\n", opts.Output, opts.Output)
}
