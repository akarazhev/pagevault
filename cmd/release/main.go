// Command release builds and packages a PageVault release using only Go tools.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"pagevault.local/pagevault/internal/buildinfo"
	"pagevault.local/pagevault/internal/release"
)

func main() {
	var opts release.Options
	flag.StringVar(&opts.Output, "output", "dist/PageVault-"+buildinfo.Version+".zip", "output release ZIP")
	flag.StringVar(&opts.Go, "go", "go", "Go compiler executable")
	flag.BoolVar(&opts.SourceOnly, "source-only", false, "package source without compiling or including hosts")
	flag.BoolVar(&opts.SkipBuild, "skip-build", false, "package the six existing bin platform hosts without compiling")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "release: unexpected positional arguments")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := release.Run(ctx, opts); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
	fmt.Printf("Created %s and %s.sha256\n", opts.Output, opts.Output)
}
