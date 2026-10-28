package release

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/akarazhev/pagevault/internal/buildinfo"
)

func project(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range map[string]string{
		"README.md": "release", "docs/VERIFICATION.md": "verified",
		"go.mod":                  "module github.com/akarazhev/pagevault\n\ngo 1.26.0\n",
		"extension/manifest.json": fmt.Sprintf(`{"version":%q}`, buildinfo.Version),
		"cmd/pagevault/main.go":   "package main", "cmd/release/main.go": "package main",
	} {
		put(t, root, name, data)
	}
	return root
}

func put(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func archive(t *testing.T, path string) map[string]*zip.File {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string]*zip.File)
	var names []string
	for _, f := range z.File {
		files[f.Name] = f
		names = append(names, f.Name)
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("unsorted entries: %v", names)
	}
	return files
}

func content(t *testing.T, f *zip.File) string {
	t.Helper()
	r, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSourceWhitelistExclusionsPermissionsAndDeterminism(t *testing.T) {
	root := project(t)
	for _, name := range []string{"LICENSE", "install.sh", "extension/ui/popup.html", "scripts/check.sh", "internal/example/example.go", "tests/example.test.cjs", "docs/ADAPTERS.md"} {
		put(t, root, name, name)
	}
	for _, name := range []string{
		"host/main.go", "secret.txt", ".git/config", "node_modules/pkg/index.js", "dist/old.zip",
		"docs/tasks/report.md", "docs/superpowers/plans/private.md", "docs/CONTRACTS.md",
		"extension/node_modules/secret.js", "extension/.git/config", "tests/__pycache__/secret.pyc",
		"tests/.test-artifacts/private.json", "scripts/legacy.py", "scripts/legacy.pyc",
		"internal/archive/private.sqlite", "internal/archive/private.sqlite-wal", "tests/private.db.backup",
		"extension/private.pem", "internal/reports/private.md", "internal/plans/private.md",
		"scripts/sdk/go", "scripts/.cache/private", "tests/archive.zip", "bin/unexpected/secret",
	} {
		put(t, root, name, "PRIVATE")
	}
	if err := os.Symlink(filepath.Join(root, "README.md"), filepath.Join(root, "extension", "linked.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "internal"), filepath.Join(root, "extension", "linked-dir")); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "dist", "release.zip")
	if err := Run(context.Background(), Options{Root: root, Output: out, SourceOnly: true}); err != nil {
		t.Fatal(err)
	}
	files := archive(t, out)
	if len(files) != 13 {
		t.Fatalf("unexpected entries (%d): %v", len(files), files)
	}
	for name, f := range files {
		if strings.Contains(content(t, f), "PRIVATE") {
			t.Fatalf("private content included: %s", name)
		}
		want := os.FileMode(0644)
		if strings.HasSuffix(name, ".sh") {
			want = 0755
		}
		if f.Mode().Perm() != want {
			t.Errorf("%s mode = %o, want %o", name, f.Mode().Perm(), want)
		}
		if !f.Modified.Equal(zipTime) {
			t.Errorf("%s time = %v", name, f.Modified)
		}
	}
	first, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), Options{Root: root, Output: out, SourceOnly: true}); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(out)
	if string(first) != string(second) {
		t.Fatal("archive bytes changed")
	}
	sum := sha256.Sum256(second)
	checksum, _ := os.ReadFile(out + ".sha256")
	if string(checksum) != hex.EncodeToString(sum[:])+"  release.zip\n" {
		t.Fatalf("invalid artifact checksum: %s", checksum)
	}
}

func TestRejectVersionMismatchAndRequiredSources(t *testing.T) {
	for _, missing := range []string{"", "README.md", "docs/VERIFICATION.md", "go.mod", "cmd/pagevault/main.go", "cmd/release/main.go"} {
		t.Run(missing, func(t *testing.T) {
			root := project(t)
			if missing == "" {
				put(t, root, "extension/manifest.json", `{"version":"0.0.0"}`)
			} else if err := os.Remove(filepath.Join(root, missing)); err != nil {
				t.Fatal(err)
			}
			if err := Run(context.Background(), Options{Root: root, Output: filepath.Join(root, "dist/release.zip"), SourceOnly: true}); err == nil {
				t.Fatal("accepted invalid project")
			}
		})
	}
}

func TestNodeVersionPinShipsWithRelease(t *testing.T) {
	root := project(t)
	put(t, root, ".nvmrc", "26.0.0\n")
	out := filepath.Join(root, "dist", "release.zip")
	if err := Run(context.Background(), Options{Root: root, Output: out, SourceOnly: true}); err != nil {
		t.Fatal(err)
	}
	file := archive(t, out)["pagevault/.nvmrc"]
	if file == nil {
		t.Fatal("release is missing the Node.js version pin")
	}
	if content(t, file) != "26.0.0\n" {
		t.Fatal("release changed the Node.js version pin")
	}
}

func TestMCPAndDistributionDocsShipWithRelease(t *testing.T) {
	root := project(t)
	for _, name := range []string{"MCP.md", "DISTRIBUTION.md"} {
		put(t, root, "docs/"+name, "document "+name)
	}
	output := filepath.Join(root, "dist", "source.zip")
	if err := Run(context.Background(), Options{Root: root, Output: output, SourceOnly: true}); err != nil {
		t.Fatal(err)
	}
	files := archive(t, output)
	for _, name := range []string{"MCP.md", "DISTRIBUTION.md"} {
		f := files["pagevault/docs/"+name]
		if f == nil || content(t, f) != "document "+name {
			t.Fatalf("missing release document %s", name)
		}
	}
}

func TestSkipBuildRequiresEveryTargetAndChecksums(t *testing.T) {
	root := project(t)
	out := filepath.Join(root, "dist/release.zip")
	for i, target := range targets {
		if err := Run(context.Background(), Options{Root: root, Output: out, SkipBuild: true}); err == nil {
			t.Fatalf("accepted only %d binaries", i)
		}
		put(t, root, target.path(), target.os+"/"+target.arch)
	}
	put(t, root, "bin/linux-amd64/extra.txt", "PRIVATE")
	if err := Run(context.Background(), Options{Root: root, Output: out, SkipBuild: true}); err != nil {
		t.Fatal(err)
	}
	files := archive(t, out)
	checksums := content(t, files["pagevault/bin/SHA256SUMS"])
	for _, target := range targets {
		f := files["pagevault/"+target.path()]
		if f == nil {
			t.Fatalf("missing %s", target.path())
		}
		if f.Mode().Perm() != 0755 {
			t.Errorf("binary mode %o", f.Mode().Perm())
		}
		sum := sha256.Sum256([]byte(content(t, f)))
		line := fmt.Sprintf("%x  %s\n", sum, strings.TrimPrefix(target.path(), "bin/"))
		if !strings.Contains(checksums, line) {
			t.Errorf("missing checksum %s", line)
		}
	}
	if files["pagevault/bin/linux-amd64/extra.txt"] != nil {
		t.Fatal("included unexpected binary sidecar")
	}
}

func TestSkipBuildRejectsSymlinkBinary(t *testing.T) {
	root := project(t)
	for _, target := range targets {
		put(t, root, target.path(), "binary")
	}
	path := filepath.Join(root, targets[0].path())
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "README.md"), path); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), Options{Root: root, Output: filepath.Join(root, "out.zip"), SkipBuild: true}); err == nil {
		t.Fatal("accepted symlink binary")
	}
}

func TestOptionalSymlinksAreExcluded(t *testing.T) {
	root := project(t)
	if err := os.Symlink(filepath.Join(root, "README.md"), filepath.Join(root, "LICENSE")); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "dist/release.zip")
	if err := Run(context.Background(), Options{Root: root, Output: out, SourceOnly: true}); err != nil {
		t.Fatal(err)
	}
	if archive(t, out)["pagevault/LICENSE"] != nil {
		t.Fatal("included optional symlink")
	}
}

func TestOutputInsideSourceDirectoryDoesNotPackageOwnChecksum(t *testing.T) {
	root := project(t)
	out := filepath.Join(root, "extension", "release.zip")
	for i := 0; i < 2; i++ {
		if err := Run(context.Background(), Options{Root: root, Output: out, SourceOnly: true}); err != nil {
			t.Fatal(err)
		}
		if archive(t, out)["pagevault/extension/release.zip.sha256"] != nil {
			t.Fatal("included output checksum")
		}
	}
}

func TestOutputThroughDirectoryAliasExcludesOwnChecksum(t *testing.T) {
	for _, name := range []string{"extension/release.zip", "extension/new/nested/release.zip"} {
		t.Run(name, func(t *testing.T) {
			root := project(t)
			alias := filepath.Join(t.TempDir(), "Проект с пробелами")
			if err := os.Symlink(root, alias); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(alias, filepath.FromSlash(name))
			for i := 0; i < 2; i++ {
				if err := Run(context.Background(), Options{Root: root, Output: out, SourceOnly: true}); err != nil {
					t.Fatal(err)
				}
				files := archive(t, out)
				if files["pagevault/"+name+".sha256"] != nil {
					t.Fatal("included output checksum through directory alias")
				}
				if content(t, files["pagevault/README.md"]) != "release" {
					t.Fatal("release source changed")
				}
			}
		})
	}
}

func TestOutputAliasCannotOverwriteReleaseSource(t *testing.T) {
	root := project(t)
	alias := filepath.Join(t.TempDir(), "project alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), Options{Root: root, Output: filepath.Join(alias, "README.md"), SourceOnly: true})
	if err == nil {
		t.Error("accepted a release source as output through a directory alias")
	}
	got, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "release" {
		t.Fatal("overwrote release source through directory alias")
	}
}

func TestArchiveReplacesSymlinkWithoutChangingTarget(t *testing.T) {
	root := project(t)
	out := filepath.Join(root, "dist", "release.zip")
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "README.md"), out); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), Options{Root: root, Output: out, SourceOnly: true}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "release" {
		t.Fatal("archive overwrote symlink target")
	}
	info, err := os.Lstat(out)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatal("archive did not replace the symlink with a regular file")
	}
	if content(t, archive(t, out)["pagevault/README.md"]) != "release" {
		t.Fatal("archive content changed")
	}
}

func TestOutputCaseAliasCannotOverwriteReleaseSource(t *testing.T) {
	root := project(t)
	out := filepath.Join(root, "readme.md")
	if _, err := os.Stat(out); os.IsNotExist(err) {
		t.Skip("case-sensitive filesystem")
	} else if err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), Options{Root: root, Output: out, SourceOnly: true}); err == nil {
		t.Error("accepted a case alias of a release source as output")
	}
	got, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "release" {
		t.Fatal("overwrote release source through a case alias")
	}
}

func TestOutputCaseAliasesExcludeOwnChecksum(t *testing.T) {
	for _, name := range []string{"EXTENSION/release.zip", "extension/RELEASE.zip"} {
		t.Run(name, func(t *testing.T) {
			root := project(t)
			if _, err := os.Stat(filepath.Join(root, "EXTENSION")); os.IsNotExist(err) {
				t.Skip("case-sensitive filesystem")
			} else if err != nil {
				t.Fatal(err)
			}
			first := filepath.Join(root, "extension", "release.zip")
			if err := Run(context.Background(), Options{Root: root, Output: first, SourceOnly: true}); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(root, filepath.FromSlash(name))
			if err := Run(context.Background(), Options{Root: root, Output: out, SourceOnly: true}); err != nil {
				t.Fatal(err)
			}
			for filename := range archive(t, out) {
				if strings.HasSuffix(filename, ".zip.sha256") {
					t.Fatal("included output checksum through a case alias")
				}
			}
		})
	}
}

func TestArchiveReplacesHardLinkWithoutChangingSource(t *testing.T) {
	root := project(t)
	out := filepath.Join(root, "release.zip")
	if err := os.Link(filepath.Join(root, "README.md"), out); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), Options{Root: root, Output: out, SourceOnly: true}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "release" {
		t.Fatal("archive overwrote source through a hard link")
	}
	if content(t, archive(t, out)["pagevault/README.md"]) != "release" {
		t.Fatal("archive content changed")
	}
}

func TestChecksumDoesNotFollowExistingSymlink(t *testing.T) {
	root := project(t)
	out := filepath.Join(root, "dist/release.zip")
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "README.md"), out+".sha256"); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), Options{Root: root, Output: out, SourceOnly: true}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "release" {
		t.Fatalf("checksum overwrote source: %q", b)
	}
}

func TestBuildInvokesEveryTargetAndStopsOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("compiler stand-in uses POSIX sh")
	}
	for _, fail := range []string{"", "linux/amd64"} {
		t.Run("fail="+fail, func(t *testing.T) {
			root := project(t)
			compiler := filepath.Join(t.TempDir(), "go compiler")
			logPath := filepath.Join(t.TempDir(), "compiler.log")
			script := `#!/bin/sh
printf '%s|%s|%s|%s\n' "$GOOS/$GOARCH" "$CGO_ENABLED" "$HOME" "$*" >> "$PAGEVAULT_TEST_COMPILER_LOG"
if [ "$GOOS/$GOARCH" = "$PAGEVAULT_TEST_COMPILER_FAIL" ]; then
  echo 'compiler intentionally failed' >&2
  exit 7
fi
while [ "$1" != '-o' ]; do shift; done
shift
printf '%s/%s' "$GOOS" "$GOARCH" > "$1"
`
			if err := os.WriteFile(compiler, []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PAGEVAULT_TEST_COMPILER_LOG", logPath)
			t.Setenv("PAGEVAULT_TEST_COMPILER_FAIL", fail)
			t.Setenv("GOOS", "wrong")
			t.Setenv("GOARCH", "wrong")
			t.Setenv("CGO_ENABLED", "1")
			home := os.Getenv("HOME")
			out := filepath.Join(root, "dist/release.zip")
			err := Run(context.Background(), Options{Root: root, Output: out, Go: compiler})
			log, readErr := os.ReadFile(logPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			lines := strings.Split(strings.TrimSpace(string(log)), "\n")
			if fail != "" {
				if err == nil || !strings.Contains(err.Error(), fail) || !strings.Contains(err.Error(), "compiler intentionally failed") {
					t.Fatalf("missing target failure: %v", err)
				}
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatalf("failed build published ZIP: %v", err)
				}
				if _, err := os.Stat(filepath.Join(root, "bin")); !os.IsNotExist(err) {
					t.Fatalf("failed build published partial hosts: %v", err)
				}
				if len(lines) != 3 {
					t.Fatalf("did not stop at failed target: %s", log)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(lines) != len(targets) {
				t.Fatalf("wrong target count: %s", log)
			}
			for i, line := range lines {
				want := targets[i].os + "/" + targets[i].arch + "|0|" + home + "|build -trimpath -buildvcs=false -ldflags=-s -w -o "
				if !strings.HasPrefix(line, want) || !strings.HasSuffix(line, " ./cmd/pagevault") {
					t.Errorf("compiler args/env incorrect: %s", line)
				}
				b, err := os.ReadFile(filepath.Join(root, targets[i].path()))
				if err != nil {
					t.Fatal(err)
				}
				if string(b) != targets[i].os+"/"+targets[i].arch {
					t.Fatalf("wrong published host: %q", b)
				}
			}
		})
	}
}

func TestReleaseChecksumDirectoryPreservesOldOutput(t *testing.T) {
	root := project(t)
	output := filepath.Join(t.TempDir(), "release.zip")
	if err := os.WriteFile(output, []byte("old artifact"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(output+".sha256", 0700); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), Options{Root: root, Output: output, SourceOnly: true}); err == nil {
		t.Fatal("expected checksum directory rejection")
	}
	got, err := os.ReadFile(output)
	if err != nil || string(got) != "old artifact" {
		t.Fatalf("old artifact lost: %q, %v", got, err)
	}
}
