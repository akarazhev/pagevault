package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akarazhev/pagevault/internal/buildinfo"
)

func writeObject(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, raw)
}

func TestPreserveRegisteredDatabaseFromCustomRuntime(t *testing.T) {
	home := filepath.Join(t.TempDir(), "Пользователь с пробелами")
	base := Options{Browser: "chrome", Platform: "Darwin", Home: home, DryRun: true}
	initial := plan(t, base)
	configPath := initial["config_path"].(string)
	manifestPath := initial["manifest_path"].(string)
	customBinary := filepath.Join(t.TempDir(), "custom runtime", "pagevault-host")
	activeDB := filepath.Join(t.TempDir(), "активный архив.sqlite3")
	staleDB := filepath.Join(home, "stale.sqlite3")
	writeObject(t, configPath, map[string]any{"schema_version": 1, "db_path": staleDB})
	writeObject(t, filepath.Join(filepath.Dir(customBinary), "pagevault.json"), map[string]any{"schema_version": 1, "db_path": activeDB})
	// A deliberately non-executable old program: adoption only reads configuration.
	mustWrite(t, customBinary, []byte("must never execute this file"))
	writeObject(t, manifestPath, map[string]any{"name": buildinfo.HostName, "type": "stdio", "path": customBinary})
	before, _ := os.ReadFile(manifestPath)
	base.PreserveRegisteredDB = true
	got := plan(t, base)
	if got["db_path"] != activeDB || got["executable_path"] != initial["executable_path"] {
		t.Fatal(got)
	}
	assertBytes(t, manifestPath, before)
	base.DBPath = filepath.Join(home, "explicit.sqlite3")
	if got := plan(t, base); got["db_path"] != base.DBPath {
		t.Fatal(got)
	}
	base.DBPath = ""
	base.PreserveRegisteredDB = false
	if got := plan(t, base); got["db_path"] != staleDB {
		t.Fatal("ordinary installation behavior changed", got)
	}
}

func TestPreserveRegisteredDatabaseWithoutManifestUsesExistingConfig(t *testing.T) {
	home := t.TempDir()
	o := Options{Browser: "chrome", Platform: "Darwin", Home: home, DryRun: true, PreserveRegisteredDB: true}
	initial := plan(t, o)
	db := filepath.Join(home, "existing.sqlite3")
	writeObject(t, initial["config_path"].(string), map[string]any{"schema_version": 1, "db_path": db})
	if got := plan(t, o); got["db_path"] != db {
		t.Fatal(got)
	}
}

func TestBadRegisteredDatabaseCannotReplaceRegistration(t *testing.T) {
	for _, variant := range []string{"json", "name", "type", "relative-executable", "missing-config", "bad-config", "schema", "relative-db", "empty-db"} {
		t.Run(variant, func(t *testing.T) {
			home := t.TempDir()
			o := Options{Browser: "chrome", Platform: "Darwin", Home: home, DryRun: true, PreserveRegisteredDB: true}
			initial := plan(t, o)
			manifestPath := initial["manifest_path"].(string)
			binary := filepath.Join(home, "custom", "pagevault-host")
			m := map[string]any{"name": buildinfo.HostName, "type": "stdio", "path": binary}
			c := map[string]any{"schema_version": 1, "db_path": filepath.Join(home, "active.sqlite3")}
			switch variant {
			case "name":
				m["name"] = "com.other.host"
			case "type":
				m["type"] = "http"
			case "relative-executable":
				m["path"] = "relative-host"
			case "schema":
				c["schema_version"] = 2
			case "relative-db":
				c["db_path"] = "relative.sqlite3"
			case "empty-db":
				c["db_path"] = ""
			}
			writeObject(t, manifestPath, m)
			if variant == "json" {
				mustWrite(t, manifestPath, []byte("not json"))
			}
			config := filepath.Join(filepath.Dir(binary), "pagevault.json")
			if variant != "missing-config" {
				writeObject(t, config, c)
			}
			if variant == "bad-config" {
				mustWrite(t, config, []byte("not json"))
			}
			before, _ := os.ReadFile(manifestPath)
			o.DryRun = false
			if _, err := Run(o); err == nil || !strings.Contains(err.Error(), "--db") {
				t.Fatal("invalid registration accepted or lacks repair instructions", err)
			}
			assertBytes(t, manifestPath, before)
			if _, err := os.Stat(initial["config_path"].(string)); !os.IsNotExist(err) {
				t.Fatal("created replacement config", err)
			}
			o.DBPath = filepath.Join(home, "explicit.sqlite3")
			o.DryRun = true
			if got := plan(t, o); got["db_path"] != o.DBPath {
				t.Fatal(got)
			}
		})
	}
}
