package release

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func extensionProject(t *testing.T) string {
	t.Helper()
	root := project(t)
	put(t, root, "LICENSE", "MIT fixture license")
	put(t, root, "extension/ui/popup.html", "extension UI")
	return root
}

func TestExtensionOnlyZIPHasStoreRootAndLicense(t *testing.T) {
	root := extensionProject(t)
	for _, name := range []string{"extension/private.sqlite3", "extension/key.pem", "extension/node_modules/secret.js"} {
		put(t, root, name, "PRIVATE")
	}
	output := filepath.Join(root, "dist", "extension.zip")
	if err := Run(context.Background(), Options{Root: root, Output: output, ExtensionOnly: true, Go: filepath.Join(root, "missing-go")}); err != nil {
		t.Fatal(err)
	}
	files := archive(t, output)
	if len(files) != 3 || files["manifest.json"] == nil || files["ui/popup.html"] == nil || files["LICENSE"] == nil {
		t.Fatal("wrong store archive root/content", files)
	}
	if content(t, files["LICENSE"]) != "MIT fixture license" {
		t.Fatal("license not preserved")
	}
	for name, file := range files {
		if bytes.Contains([]byte(content(t, file)), []byte("PRIVATE")) {
			t.Fatal("private file included", name)
		}
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	checksum, err := os.ReadFile(output + ".sha256")
	if err != nil || string(checksum) != fmt.Sprintf("%x  extension.zip\n", digest) {
		t.Fatal(string(checksum), err)
	}
}

func TestExtensionOnlyFlagsAreExclusive(t *testing.T) {
	for _, opts := range []Options{{ExtensionOnly: true, SourceOnly: true}, {ExtensionOnly: true, SkipBuild: true}} {
		if err := Run(context.Background(), opts); err == nil {
			t.Fatal("accepted conflicting modes", opts)
		}
	}
}

func TestExtensionOnlyProtectsAllSourcesThroughAliases(t *testing.T) {
	root := extensionProject(t)
	alias := filepath.Join(t.TempDir(), "root alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{filepath.Join(root, "README.md"), filepath.Join(alias, "README.md"), filepath.Join(root, "extension", "manifest.json")} {
		before, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		if err := Run(context.Background(), Options{Root: root, Output: output, ExtensionOnly: true}); err == nil {
			t.Fatal("overwrote source", output)
		}
		after, err := os.ReadFile(output)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("source changed", output, err)
		}
	}
}

func TestExtensionOnlyDoesNotIncludeOwnOutputs(t *testing.T) {
	root := extensionProject(t)
	alias := filepath.Join(t.TempDir(), "extension alias")
	if err := os.Symlink(filepath.Join(root, "extension"), alias); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(alias, "bundle.zip")
	opts := Options{Root: root, Output: output, ExtensionOnly: true}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("extension ZIP includes changing output", err)
	}
	files := archive(t, output)
	if len(files) != 3 {
		t.Fatal("included own ZIP/checksum", files)
	}
}
