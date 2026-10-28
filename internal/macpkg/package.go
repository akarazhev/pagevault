// Package macpkg builds native, per-user macOS installer packages.
package macpkg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"pagevault.local/pagevault/internal/buildinfo"
)

type Options struct {
	Root, Output, Go                       string
	Unsigned                               bool
	ApplicationIdentity, InstallerIdentity string
}

type Artifact struct{ Path, SHA256 string }

// Build compiles current sources for both architectures and stages every part
// privately. Existing output is replaced only after the native tools succeed.
func Build(ctx context.Context, opts Options) (Artifact, error) {
	if opts.Unsigned {
		if opts.ApplicationIdentity != "" || opts.InstallerIdentity != "" {
			return Artifact{}, errors.New("--unsigned cannot be combined with signing identities")
		}
	} else if opts.ApplicationIdentity == "" || opts.InstallerIdentity == "" {
		return Artifact{}, errors.New("provide both Developer ID Application and Installer identities, or use --unsigned for local testing")
	}
	if opts.Root == "" {
		opts.Root = "."
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return Artifact{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Artifact{}, err
	}
	if opts.Output == "" {
		suffix := ""
		if opts.Unsigned {
			suffix = "-unsigned"
		}
		opts.Output = filepath.Join("dist", "PageVault-"+buildinfo.Version+"-macOS"+suffix+".pkg")
	}
	if !filepath.IsAbs(opts.Output) {
		opts.Output = filepath.Join(root, opts.Output)
	}
	output := filepath.Clean(opts.Output)
	if err := safeOutput(root, output, opts.Unsigned); err != nil {
		return Artifact{}, err
	}
	if runtime.GOOS != "darwin" {
		return Artifact{}, errors.New("building PKG requires macOS with pkgbuild, productbuild, lipo and codesign")
	}
	if opts.Go == "" {
		opts.Go = "go"
	}
	for _, file := range []string{"go.mod", "cmd/pagevault/main.go", "LICENSE", "THIRD_PARTY_NOTICES.md", "extension/manifest.json"} {
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			return Artifact{}, err
		}
		if !info.Mode().IsRegular() {
			return Artifact{}, fmt.Errorf("required source is not a regular file: %s", file)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(root, "extension", "manifest.json"))
	if err != nil {
		return Artifact{}, err
	}
	var identity struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(manifest, &identity); err != nil {
		return Artifact{}, err
	}
	if identity.Version != buildinfo.Version {
		return Artifact{}, fmt.Errorf("manifest version %q differs from package version %q", identity.Version, buildinfo.Version)
	}
	stage, err := os.MkdirTemp("", "pagevault-macpkg-*")
	if err != nil {
		return Artifact{}, err
	}
	defer os.RemoveAll(stage)
	invoke := func(program string, env []string, args ...string) error {
		cmd := exec.CommandContext(ctx, program, args...)
		cmd.Dir = root
		if env != nil {
			cmd.Env = append(os.Environ(), env...)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("macOS package %s: %w\n%s", filepath.Base(program), err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	arches := []string{"amd64", "arm64"}
	for _, arch := range arches {
		if err := invoke(opts.Go, []string{"CGO_ENABLED=0", "GOOS=darwin", "GOARCH=" + arch}, "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w", "-o", filepath.Join(stage, "host-"+arch), "./cmd/pagevault"); err != nil {
			return Artifact{}, err
		}
	}
	payload := filepath.Join(stage, "payload")
	base := filepath.Join(payload, "Library", "Application Support", "PageVault", "Installer")
	if err := os.MkdirAll(base, 0755); err != nil {
		return Artifact{}, err
	}
	binary := filepath.Join(base, "pagevault-host")
	if err := invoke("/usr/bin/lipo", nil, "-create", filepath.Join(stage, "host-amd64"), filepath.Join(stage, "host-arm64"), "-output", binary); err != nil {
		return Artifact{}, err
	}
	if opts.Unsigned {
		err = invoke("/usr/bin/codesign", nil, "--force", "--sign", "-", "--timestamp=none", binary)
	} else {
		err = invoke("/usr/bin/codesign", nil, "--force", "--sign", opts.ApplicationIdentity, "--options", "runtime", "--timestamp", binary)
	}
	if err != nil {
		return Artifact{}, err
	}
	for _, file := range []string{"LICENSE", "THIRD_PARTY_NOTICES.md"} {
		raw, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			return Artifact{}, err
		}
		if err := os.WriteFile(filepath.Join(base, file), raw, 0644); err != nil {
			return Artifact{}, err
		}
	}
	if err := invoke("/usr/bin/pkgbuild", nil, "--root", payload, "--identifier", "com.pagevault.host.core", "--version", buildinfo.Version, "--install-location", "/", filepath.Join(stage, "core.pkg")); err != nil {
		return Artifact{}, err
	}
	for _, browser := range browsers {
		scripts := filepath.Join(stage, "scripts-"+browser.ID)
		if err := os.Mkdir(scripts, 0700); err != nil {
			return Artifact{}, err
		}
		if err := os.WriteFile(filepath.Join(scripts, "postinstall"), []byte(postinstall(browser.ID)), 0755); err != nil {
			return Artifact{}, err
		}
		if err := invoke("/usr/bin/pkgbuild", nil, "--nopayload", "--scripts", scripts, "--identifier", "com.pagevault.host."+browser.ID, "--version", buildinfo.Version, "--install-location", "/", filepath.Join(stage, browser.ID+".pkg")); err != nil {
			return Artifact{}, err
		}
	}
	resources := filepath.Join(stage, "resources")
	if err := os.Mkdir(resources, 0700); err != nil {
		return Artifact{}, err
	}
	if err := writeResources(resources, opts.Unsigned); err != nil {
		return Artifact{}, err
	}
	distribution := filepath.Join(stage, "Distribution")
	if err := os.WriteFile(distribution, distributionXML(), 0600); err != nil {
		return Artifact{}, err
	}
	product := filepath.Join(stage, "PageVault.pkg")
	args := []string{"--distribution", distribution, "--package-path", stage, "--resources", resources}
	if !opts.Unsigned {
		args = append(args, "--sign", opts.InstallerIdentity, "--timestamp")
	}
	args = append(args, product)
	if err := invoke("/usr/bin/productbuild", nil, args...); err != nil {
		return Artifact{}, err
	}
	if err := ctx.Err(); err != nil {
		return Artifact{}, err
	}
	return publish(product, output)
}

// Check ancestor identity so a symlink/case alias cannot redirect output into
// source directories. Final-component symlinks are safely replaced by rename.
func safeOutput(root, output string, unsigned bool) error {
	if filepath.Ext(output) != ".pkg" {
		return errors.New("package output must end in .pkg")
	}
	if unsigned && !strings.HasSuffix(output, "-unsigned.pkg") {
		return errors.New("local test output must end in -unsigned.pkg")
	}
	parent := filepath.Dir(output)
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			parent = resolved
			break
		}
		if !os.IsNotExist(err) || filepath.Dir(parent) == parent {
			return err
		}
		parent = filepath.Dir(parent)
	}
	if info, err := os.Stat(parent); err != nil {
		return err
	} else if !info.IsDir() {
		return errors.New("package output parent is not a directory")
	}
	for _, source := range []string{"cmd", "internal", "extension", "docs", "scripts", "tests", ".git"} {
		info, err := os.Stat(filepath.Join(root, source))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for current := parent; ; current = filepath.Dir(current) {
			ancestor, err := os.Stat(current)
			if err == nil && os.SameFile(info, ancestor) {
				return fmt.Errorf("package output cannot be inside source directory %s", source)
			}
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if filepath.Dir(current) == current {
				break
			}
		}
	}
	return nil
}

func publish(source, output string) (Artifact, error) {
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return Artifact{}, err
	}
	in, err := os.Open(source)
	if err != nil {
		return Artifact{}, err
	}
	defer in.Close()
	f, err := os.CreateTemp(filepath.Dir(output), ".pagevault-package-*.pkg")
	if err != nil {
		return Artifact{}, err
	}
	defer os.Remove(f.Name())
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, hash), in)
	closeErr := f.Close()
	if copyErr != nil {
		return Artifact{}, copyErr
	}
	if closeErr != nil {
		return Artifact{}, closeErr
	}
	if err := os.Chmod(f.Name(), 0644); err != nil {
		return Artifact{}, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	checksum, err := os.CreateTemp(filepath.Dir(output), ".pagevault-package-checksum-*")
	if err != nil {
		return Artifact{}, err
	}
	defer os.Remove(checksum.Name())
	_, writeErr := fmt.Fprintf(checksum, "%s  %s\n", digest, filepath.Base(output))
	closeErr = checksum.Close()
	if writeErr != nil {
		return Artifact{}, writeErr
	}
	if closeErr != nil {
		return Artifact{}, closeErr
	}
	if err := os.Chmod(checksum.Name(), 0644); err != nil {
		return Artifact{}, err
	}
	if err := os.Rename(f.Name(), output); err != nil {
		return Artifact{}, err
	}
	if err := os.Rename(checksum.Name(), output+".sha256"); err != nil {
		return Artifact{}, err
	}
	return Artifact{Path: output, SHA256: digest}, nil
}
