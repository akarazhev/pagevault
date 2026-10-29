package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/akarazhev/pagevault/internal/archive"
	"github.com/akarazhev/pagevault/internal/buildinfo"
)

func TestPackagePreflightRejectsUnselectedRegisteredArchiveAliases(t *testing.T) {
	for _, name := range []string{"pagevault-host", "LICENSE", "THIRD_PARTY_NOTICES.md"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			base := filepath.Join(home, "Library", "Application Support", "PageVault", "Installer")
			if err := os.MkdirAll(base, 0755); err != nil {
				t.Fatal(err)
			}
			db := filepath.Join(base, name)
			w, err := archive.Open(db)
			if err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(db)
			if err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(home, "alias")
			if err := os.Symlink(base, alias); err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(home, "Library", "Application Support", "PageVault", "host", "brave", "pagevault.json")
			if err := os.MkdirAll(filepath.Dir(config), 0755); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(map[string]any{"schema_version": 1, "db_path": filepath.Join(alias, name)})
			if err := os.WriteFile(config, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if err := CheckPackagePayload(home); err == nil {
				t.Fatal("accepted unselected browser archive")
			}
			after, err := os.ReadFile(db)
			if err != nil || string(after) != string(before) {
				t.Fatalf("archive changed: %v", err)
			}
		})
	}
}

func TestPackagePreflightRejectsActiveUnselectedRegistration(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(home, "Library", "Application Support", "PageVault", "Installer")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(base, "LICENSE")
	alias := filepath.Join(home, "alias")
	if err := os.Symlink(base, alias); err != nil {
		t.Fatal(err)
	}
	runtime := filepath.Join(home, "custom runtime")
	if err := os.MkdirAll(runtime, 0755); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(map[string]any{"schema_version": 1, "db_path": filepath.Join(alias, "LICENSE")})
	if err := os.WriteFile(filepath.Join(runtime, "pagevault.json"), config, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(home, "Library", "Application Support", "Microsoft Edge", "NativeMessagingHosts", buildinfo.HostName+".json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"name": buildinfo.HostName, "type": "stdio", "path": filepath.Join(runtime, "old-host")})
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckPackagePayload(home); err == nil {
		t.Fatal("accepted registered archive at payload destination")
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatalf("preflight created archive: %v", err)
	}
}

func TestPackagePreflightProtectsFirefoxRegisteredArchive(t *testing.T) {
	home := t.TempDir()
	runtime := filepath.Join(home, "custom firefox runtime")
	db := filepath.Join(home, "Library", "Application Support", "PageVault", "Installer", "LICENSE")
	config, _ := json.Marshal(map[string]any{"schema_version": 1, "db_path": db})
	mustWrite(t, filepath.Join(runtime, "pagevault.json"), config)
	manifest := filepath.Join(home, "Library", "Application Support", "Mozilla", "NativeMessagingHosts", "com.pagevault.host.json")
	data, _ := json.Marshal(map[string]any{"name": "com.pagevault.host", "type": "stdio", "path": filepath.Join(runtime, "pagevault-host")})
	mustWrite(t, manifest, data)
	if err := CheckPackagePayload(home); err == nil {
		t.Fatal("Firefox archive could be overwritten by installer payload")
	}
}

func TestPackagePreflightRejectsFallbackConfigWithAbsentPayload(t *testing.T) {
	for _, name := range []string{"pagevault-host", "LICENSE", "THIRD_PARTY_NOTICES.md"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			base := filepath.Join(home, "Library", "Application Support", "PageVault", "Installer")
			if err := os.MkdirAll(base, 0755); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(home, "alias")
			if err := os.Symlink(base, alias); err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(home, "Library", "Application Support", "PageVault", "host", "brave", "pagevault.json")
			if err := os.MkdirAll(filepath.Dir(config), 0755); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(map[string]any{"schema_version": 1, "db_path": filepath.Join(alias, name)})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(config, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if err := CheckPackagePayload(home); err == nil {
				t.Fatal("accepted fallback archive path at absent payload destination")
			}
			if _, err := os.Stat(filepath.Join(base, name)); !os.IsNotExist(err) {
				t.Fatalf("preflight changed destination: %v", err)
			}
		})
	}
}
