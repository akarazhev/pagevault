// Command package-macos creates the per-user macOS Installer product.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/akarazhev/pagevault/internal/macpkg"
)

func main() {
	var opts macpkg.Options
	flag.StringVar(&opts.Output, "output", "", "Output .pkg (unsigned names end in -unsigned.pkg)")
	flag.StringVar(&opts.Go, "go", "go", "Go compiler executable")
	flag.BoolVar(&opts.Unsigned, "unsigned", false, "Build an explicitly labelled package without Developer ID")
	flag.StringVar(&opts.ApplicationIdentity, "sign-app", "", "Developer ID Application identity")
	flag.StringVar(&opts.InstallerIdentity, "sign-installer", "", "Developer ID Installer identity")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "package-macos: unexpected positional arguments")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	artifact, err := macpkg.Build(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "package-macos:", err)
		os.Exit(1)
	}
	fmt.Printf("Created %s and %s.sha256\n", artifact.Path, artifact.Path)
}
