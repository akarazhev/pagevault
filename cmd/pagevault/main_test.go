package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pagevault.local/pagevault/internal/buildinfo"
)

func TestDatabaseConfigPrecedence(t *testing.T) {
	t.Setenv("PAGEVAULT_DB", "")
	dir := t.TempDir()
	exe := filepath.Join(dir, "pagevault-host")
	config := filepath.Join(dir, "pagevault.json")
	configDB, envDB, explicitDB := filepath.Join(dir, "from-config.sqlite3"), filepath.Join(dir, "from-env.sqlite3"), filepath.Join(dir, "explicit.sqlite3")
	if got, err := databasePath("", exe); err != nil || got != "" {
		t.Fatalf("uninstalled default: %q %v", got, err)
	}
	encoded, _ := json.Marshal(map[string]any{"schema_version": 1, "db_path": configDB})
	if err := os.WriteFile(config, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := databasePath("", exe); err != nil || got != configDB {
		t.Fatalf("config: %q %v", got, err)
	}
	t.Setenv("PAGEVAULT_DB", envDB)
	if got, err := databasePath("", exe); err != nil || got != envDB {
		t.Fatalf("env: %q %v", got, err)
	}
	if got, err := databasePath(explicitDB, exe); err != nil || got != explicitDB {
		t.Fatalf("explicit: %q %v", got, err)
	}
	t.Setenv("PAGEVAULT_DB", "")
	if err := os.WriteFile(config, []byte(`{"schema_version":99,"db_path":"wrong"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := databasePath("", exe); err == nil {
		t.Fatal("invalid configuration silently accepted")
	}
}

func TestBrowserArgumentsDoNotOverrideDatabase(t *testing.T) {
	args := []string{"chrome-extension://ijkioljfdaaplhaldbdpabckkfcieiee/", "--parent-window=17", "--db", "/tmp/База с пробелами.sqlite3"}
	db, diagnose, err := parseServeArgs(args, &bytes.Buffer{})
	if err != nil || diagnose || db != "/tmp/База с пробелами.sqlite3" {
		t.Fatalf("%q %v %v", db, diagnose, err)
	}
	if _, _, err := parseServeArgs([]string{"unexpected-file"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unexpected positional argument accepted")
	}
}

func TestVersionDoesNotOpenDatabase(t *testing.T) {
	t.Setenv("PAGEVAULT_DB", filepath.Join(t.TempDir(), "missing", "db.sqlite3"))
	var out, errors bytes.Buffer
	if code := run([]string{"version"}, strings.NewReader(""), &out, &errors, "unused"); code != 0 {
		t.Fatalf("%d %s", code, errors.String())
	}
	if !strings.Contains(out.String(), buildinfo.Version) {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(os.Getenv("PAGEVAULT_DB")); !os.IsNotExist(err) {
		t.Fatal("version touched database")
	}
}

func TestDiagnoseReadsAdjacentConfiguration(t *testing.T) {
	t.Setenv("PAGEVAULT_DB", "")
	dir := t.TempDir()
	database := filepath.Join(dir, "архив.sqlite3")
	config, _ := json.Marshal(map[string]any{"schema_version": 1, "db_path": database})
	if err := os.WriteFile(filepath.Join(dir, "pagevault.json"), config, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errors bytes.Buffer
	if code := run([]string{"diagnose"}, strings.NewReader(""), &out, &errors, filepath.Join(dir, "pagevault-host")); code != 0 {
		t.Fatalf("%d %s", code, errors.String())
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["db_path"] != database || result["ok"] != true {
		t.Fatalf("%v", result)
	}
}

func TestInstallDryRunLeavesDestinationUntouched(t *testing.T) {
	home := filepath.Join(t.TempDir(), "new-home")
	var out, errors bytes.Buffer
	code := run([]string{"install", "--browser", "chrome", "--home", home, "--dry-run"}, strings.NewReader(""), &out, &errors, filepath.Join(t.TempDir(), "pagevault-host"))
	if code != 0 {
		t.Fatalf("%d %s", code, errors.String())
	}
	var plan map[string]any
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan["browser"] != "chrome" {
		t.Fatalf("%v", plan)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("dry-run modified home")
	}
}
