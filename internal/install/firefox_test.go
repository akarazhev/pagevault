package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFirefoxNativeRegistrationPathsAndIdentity(t *testing.T) {
	home := filepath.Join(t.TempDir(), "Пользователь с пробелами")
	for _, system := range []string{"Linux", "Darwin", "Windows"} {
		p := plan(t, Options{Browser: "firefox", Platform: system, Home: home, DryRun: true})
		m := manifest(p)
		if m["allowed_origins"] != nil {
			t.Fatal("Firefox allows Chromium origin", m)
		}
		ids := m["allowed_extensions"].([]string)
		if len(ids) != 1 || ids[0] != "pagevault@pagevault.local" {
			t.Fatal(ids)
		}
		want := filepath.Join(home, ".mozilla", "native-messaging-hosts", "com.pagevault.host.json")
		if system == "Darwin" {
			want = filepath.Join(home, "Library", "Application Support", "Mozilla", "NativeMessagingHosts", "com.pagevault.host.json")
		}
		if system == "Windows" {
			want = filepath.Join(home, "AppData", "Local", "PageVault", "NativeMessagingHosts", "firefox", "com.pagevault.host.json")
			if p["registry_key"] != `Software\Mozilla\NativeMessagingHosts\com.pagevault.host` {
				t.Fatal(p)
			}
		}
		if p["manifest_path"] != want {
			t.Fatalf("%s: %v != %s", system, p["manifest_path"], want)
		}
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("dry run wrote files", err)
	}
}

func TestFirefoxExplicitAndManifestExtensionIDs(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "manifest.json")
	mustWrite(t, path, []byte(`{"browser_specific_settings":{"gecko":{"id":"custom@example.org"}}}`))
	p := plan(t, Options{Browser: "firefox", Home: home, ExtensionManifest: path, DryRun: true})
	if manifest(p)["allowed_extensions"].([]string)[0] != "custom@example.org" {
		t.Fatal(p)
	}
	for _, id := range []string{"@example", "{12345678-1234-1234-1234-123456789abc}"} {
		p := plan(t, Options{Browser: "firefox", Home: home, ExtensionID: id, ExtensionManifest: "missing", DryRun: true})
		if manifest(p)["allowed_extensions"].([]string)[0] != id {
			t.Fatal(p)
		}
	}
	for _, bad := range []string{"", "no-at-sign", "x@y/other", "x@y\n", "chrome-extension://abc/", "{not-a-uuid}"} {
		encoded, _ := json.Marshal(map[string]any{"browser_specific_settings": map[string]any{"gecko": map[string]any{"id": bad}}})
		mustWrite(t, path, encoded)
		if _, err := Run(Options{Browser: "firefox", Home: home, ExtensionManifest: path, DryRun: true}); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestFirefoxUpgradeAndUninstallPreserveArchiveAndChromium(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows registration writes the real registry")
	}
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "installer")
	mustWrite(t, source, []byte("first executable"))
	db := filepath.Join(home, "Данные", "archive.sqlite3")
	mustWrite(t, db, []byte("preserve private archive"))
	chrome := plan(t, Options{Browser: "chrome", Home: home, ExecutablePath: source})
	opts := Options{Browser: "firefox", Home: home, ExecutablePath: source, DBPath: db}
	p := plan(t, opts)
	mustWrite(t, source, []byte("new executable"))
	opts.DBPath = ""
	opts.RuntimeDir = filepath.Join(home, "new runtime")
	opts.PreserveRegisteredDB = true
	upgraded := plan(t, opts)
	if upgraded["db_path"] != db {
		t.Fatal(upgraded)
	}
	opts.Uninstall = true
	plan(t, opts)
	assertBytes(t, db, []byte("preserve private archive"))
	assertBytes(t, chrome["executable_path"].(string), []byte("first executable"))
	if _, err := os.Stat(chrome["manifest_path"].(string)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p["manifest_path"].(string)); !os.IsNotExist(err) {
		t.Fatal("Firefox registration retained", err)
	}
}
