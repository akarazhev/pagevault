// Package release builds the supported PageVault hosts and a reproducible release ZIP.
package release

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/akarazhev/pagevault/internal/artifact"
	"github.com/akarazhev/pagevault/internal/buildinfo"
)

// Options controls release creation. Root defaults to the current directory.
// SkipBuild requires all six hosts in Root/bin; SourceOnly excludes hosts entirely.
type Options struct {
	Root          string
	Output        string
	Go            string
	SourceOnly    bool
	SkipBuild     bool
	ExtensionOnly bool
}

type target struct{ os, arch string }

var targets = []target{
	{"darwin", "amd64"}, {"darwin", "arm64"},
	{"linux", "amd64"}, {"linux", "arm64"},
	{"windows", "amd64"}, {"windows", "arm64"},
}

func (t target) path() string {
	name := "pagevault-host"
	if t.os == "windows" {
		name += ".exe"
	}
	return "bin/" + t.os + "-" + t.arch + "/" + name
}

var zipTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

var rootFiles = []string{
	"README.md", "LICENSE", "THIRD_PARTY_NOTICES.md", "go.mod", "go.sum",
	"package.json", "package-lock.json", ".gitignore", ".nvmrc", "install.sh",
	"install-windows.cmd", "install-windows.ps1", "CONTRIBUTING.md", "SECURITY.md", "CHANGELOG.md",
	".editorconfig", ".prettierignore", ".prettierrc.json", "eslint.config.js",
}

var docFiles = []string{
	"ARCHITECTURE.md", "ADAPTERS.md", "SCHEMA.md", "NATIVE_PROTOCOL.md",
	"VERIFICATION.md", "UPGRADING.md", "MCP.md", "DISTRIBUTION.md", "INSTALLATION.md", "PRIVACY.md", "CHROME_WEB_STORE.md", "PRODUCTION_READINESS.md", "extension-id.txt",
}

// Run validates source identity, optionally builds all supported platforms, and
// writes Output and Output.sha256. Failed builds never produce a release ZIP.
func Run(ctx context.Context, opts Options) error {
	if opts.SourceOnly && opts.SkipBuild {
		return fmt.Errorf("-source-only and -skip-build are mutually exclusive")
	}
	if opts.ExtensionOnly && (opts.SourceOnly || opts.SkipBuild) {
		return fmt.Errorf("-extension-only cannot be combined with -source-only or -skip-build")
	}
	if opts.Root == "" {
		opts.Root = "."
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	if err := validate(root); err != nil {
		return err
	}
	if opts.Output == "" {
		name := "PageVault-" + buildinfo.Version + ".zip"
		if opts.ExtensionOnly {
			name = "PageVault-Extension-" + buildinfo.Version + ".zip"
		}
		opts.Output = filepath.Join("dist", name)
	}
	if !filepath.IsAbs(opts.Output) {
		opts.Output = filepath.Join(root, opts.Output)
	}
	opts.Output = filepath.Clean(opts.Output)
	if opts.Go == "" {
		opts.Go = "go"
	}
	if !opts.SourceOnly && !opts.SkipBuild && !opts.ExtensionOnly {
		if err := build(ctx, root, opts.Go); err != nil {
			return err
		}
	}
	entries, err := sources(root)
	if err != nil {
		return err
	}
	if opts.ExtensionOnly {
		if _, err := regularFile(root, "LICENSE"); err != nil {
			return fmt.Errorf("extension license: %w", err)
		}
	}
	if !opts.SourceOnly && !opts.ExtensionOnly {
		var checksums strings.Builder
		for _, target := range targets {
			rel := target.path()
			path, err := regularFile(root, rel)
			if err != nil {
				return fmt.Errorf("required host %s: %w", rel, err)
			}
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if info.Size() == 0 {
				return fmt.Errorf("required host %s is empty", rel)
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			h := sha256.New()
			_, copyErr := io.Copy(h, f)
			closeErr := f.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			fmt.Fprintf(&checksums, "%x  %s\n", h.Sum(nil), strings.TrimPrefix(rel, "bin/"))
			entries = append(entries, entry{name: rel, path: path, mode: 0755})
		}
		entries = append(entries, entry{name: "bin/SHA256SUMS", data: []byte(checksums.String()), mode: 0644})
	}
	filtered := entries[:0]
	for _, entry := range entries {
		if entry.path == "" {
			filtered = append(filtered, entry)
			continue
		}
		overlap, err := sameFileEntry(entry.path, opts.Output)
		if err != nil {
			return fmt.Errorf("check output overlap: %w", err)
		}
		if overlap {
			return fmt.Errorf("output overlaps release source: %s", entry.name)
		}
		checksum, err := sameFileEntry(entry.path, opts.Output+".sha256")
		if err != nil {
			return fmt.Errorf("check output checksum: %w", err)
		}
		if !checksum {
			filtered = append(filtered, entry)
		}
	}
	entries = filtered
	prefix := "pagevault/"
	if opts.ExtensionOnly {
		prefix = ""
		only := entries[:0]
		for _, e := range entries {
			if strings.HasPrefix(e.name, "extension/") {
				e.name = strings.TrimPrefix(e.name, "extension/")
				only = append(only, e)
			} else if e.name == "LICENSE" {
				only = append(only, e)
			}
		}
		entries = only
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	return writeZIP(opts.Output, entries, prefix)
}

// Match aliases of one directory entry while allowing replacement of separate
// hard links. Lstat preserves replacement semantics for final-component symlinks.
func sameFileEntry(a, b string) (bool, error) {
	if a == b {
		return true, nil
	}
	first, err := os.Lstat(a)
	if err != nil {
		return false, err
	}
	second, err := os.Lstat(b)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !os.SameFile(first, second) {
		return false, nil
	}
	parentA, err := os.Stat(filepath.Dir(a))
	if err != nil {
		return false, err
	}
	parentB, err := os.Stat(filepath.Dir(b))
	if err != nil {
		return false, err
	}
	if !os.SameFile(parentA, parentB) {
		return false, nil
	}
	nameA, nameB := filepath.Base(a), filepath.Base(b)
	if nameA == nameB {
		return true, nil
	}
	entries, err := os.ReadDir(filepath.Dir(a))
	if err != nil {
		return false, err
	}
	var exactA, exactB bool
	for _, entry := range entries {
		exactA = exactA || entry.Name() == nameA
		exactB = exactB || entry.Name() == nameB
	}
	return !(exactA && exactB), nil
}

func validate(root string) error {
	for _, rel := range []string{"README.md", "docs/VERIFICATION.md", "go.mod", "extension/manifest.json", "cmd/pagevault/main.go", "cmd/release/main.go"} {
		if _, err := regularFile(root, rel); err != nil {
			return fmt.Errorf("required source %s: %w", rel, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(root, "extension", "manifest.json"))
	if err != nil {
		return err
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		return fmt.Errorf("extension manifest: %w", err)
	}
	if manifest.Version != buildinfo.Version {
		return fmt.Errorf("manifest version %q differs from buildinfo version %q", manifest.Version, buildinfo.Version)
	}
	b, err = os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return err
	}
	if !strings.Contains("\n"+string(b), "\nmodule github.com/akarazhev/pagevault\n") {
		return fmt.Errorf("go.mod must declare module github.com/akarazhev/pagevault")
	}
	return nil
}

// regularFile rejects symlinks in every path component, including parent dirs.
func regularFile(root, rel string) (string, error) {
	path := root
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("unsafe relative path %q", rel)
		}
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink forbidden: %s", rel)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return "", fmt.Errorf("not a directory: %s", path)
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return "", fmt.Errorf("not a regular file: %s", rel)
		}
	}
	return path, nil
}

func build(ctx context.Context, root, compiler string) error {
	tmp, err := os.MkdirTemp("", "pagevault-release-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for _, target := range targets {
		dest := filepath.Join(tmp, filepath.FromSlash(target.path()))
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, compiler, "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w", "-o", dest, "./cmd/pagevault")
		cmd.Dir = root
		for _, value := range os.Environ() {
			key, _, _ := strings.Cut(value, "=")
			if key != "GOOS" && key != "GOARCH" && key != "CGO_ENABLED" {
				cmd.Env = append(cmd.Env, value)
			}
		}
		cmd.Env = append(cmd.Env, "GOOS="+target.os, "GOARCH="+target.arch, "CGO_ENABLED=0")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("build %s/%s: %w\n%s", target.os, target.arch, err, output)
		}
		if _, err := regularFile(tmp, target.path()); err != nil {
			return fmt.Errorf("build %s/%s produced no host: %w", target.os, target.arch, err)
		}
	}
	// Publish hosts only after every target compiled successfully.
	for _, target := range targets {
		rel := target.path()
		dest := filepath.Join(root, filepath.FromSlash(rel))
		for _, dir := range []string{"bin", "bin/" + target.os + "-" + target.arch} {
			path := filepath.Join(root, filepath.FromSlash(dir))
			if err := os.Mkdir(path, 0755); err != nil && !os.IsExist(err) {
				return err
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("unsafe binary directory: %s", dir)
			}
		}
		f, err := os.Open(filepath.Join(tmp, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		file, err := os.CreateTemp(filepath.Dir(dest), ".pagevault-host-*")
		if err != nil {
			f.Close()
			return err
		}
		_, copyErr := io.Copy(file, f)
		f.Close()
		if copyErr == nil {
			copyErr = file.Chmod(0755)
		}
		closeErr := file.Close()
		if copyErr == nil {
			copyErr = closeErr
		}
		if copyErr == nil {
			copyErr = os.Rename(file.Name(), dest)
		}
		os.Remove(file.Name())
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

type entry struct {
	name, path string
	data       []byte
	mode       fs.FileMode
}

func excluded(name string, dir bool) bool {
	lower := strings.ToLower(name)
	if dir {
		switch lower {
		case ".git", "node_modules", "__pycache__", ".test-artifacts", "dist", "host", "reports", "plans", "sdk", "go-sdk", ".cache", "cache", "caches", "gopath", "gocache":
			return true
		}
	}
	for _, suffix := range []string{".py", ".pyc", ".pem", ".zip", ".tar", ".gz", ".tgz", ".7z", ".rar", ".pkg", ".pkg.sha256"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return strings.Contains(lower, ".sqlite") || strings.Contains(lower, ".db")
}

func sources(root string) ([]entry, error) {
	var entries []entry
	add := func(rel string) error {
		path, err := regularFile(root, rel)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0644)
		if strings.HasSuffix(rel, ".sh") {
			mode = 0755
		}
		entries = append(entries, entry{name: rel, path: path, mode: mode})
		return nil
	}
	for _, rel := range rootFiles {
		if info, err := os.Lstat(filepath.Join(root, rel)); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		} else if !info.Mode().IsRegular() {
			continue
		}
		if err := add(rel); err != nil {
			return nil, err
		}
	}
	for _, name := range docFiles {
		rel := "docs/" + name
		if info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		} else if !info.Mode().IsRegular() {
			continue
		}
		if err := add(rel); err != nil {
			return nil, err
		}
	}
	for _, dir := range []string{"extension", "cmd", "internal", "tests", "scripts", ".github"} {
		path := filepath.Join(root, dir)
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if excluded(d.Name(), d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			return add(filepath.ToSlash(rel))
		})
		if err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func writeZIP(output string, entries []entry, prefix string) error {
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(output), ".pagevault-release-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	z := zip.NewWriter(f)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: prefix + entry.name, Method: zip.Deflate, Modified: zipTime}
		header.SetMode(entry.mode)
		w, err := z.CreateHeader(header)
		if err == nil {
			if entry.path == "" {
				_, err = w.Write(entry.data)
			} else {
				in, openErr := os.Open(entry.path)
				err = openErr
				if err == nil {
					_, err = io.Copy(w, in)
					closeErr := in.Close()
					if err == nil {
						err = closeErr
					}
				}
			}
		}
		if err != nil {
			z.Close()
			f.Close()
			return err
		}
	}
	if err := z.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_, err = artifact.PublishPair(f.Name(), output)
	return err
}
