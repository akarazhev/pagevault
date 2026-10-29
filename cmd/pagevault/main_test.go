package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akarazhev/pagevault/internal/buildinfo"
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

func TestFirefoxStartupMetadataDoesNotOverrideDatabase(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "Нативный manifest.json")
	for _, id := range []string{"pagevault@pagevault.local", "custom@example.org", "{12345678-1234-1234-1234-123456789abc}"} {
		db, diagnose, err := parseServeArgs([]string{manifest, id}, &bytes.Buffer{})
		if err != nil || diagnose || db != "" {
			t.Fatalf("%q %v %v", db, diagnose, err)
		}
	}
	for _, args := range [][]string{{manifest}, {manifest, "--diagnose"}, {manifest, "not-an-id"}, {"relative.json", "custom@example.org"}, {manifest, "pagevault@pagevault.local", "extra"}} {
		if _, _, err := parseServeArgs(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted malformed metadata %v", args)
		}
	}
}

func TestFirefoxLaunchServesFramedProtocolUsingConfiguredArchive(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "Архив.sqlite3")
	t.Setenv("PAGEVAULT_DB", "")
	config, _ := json.Marshal(map[string]any{"schema_version": 1, "db_path": db})
	if err := os.WriteFile(filepath.Join(dir, "pagevault.json"), config, 0600); err != nil {
		t.Fatal(err)
	}
	request := []byte(`{"id":"firefox-ping","type":"ping"}`)
	var input, output, errors bytes.Buffer
	if err := binary.Write(&input, binary.NativeEndian, uint32(len(request))); err != nil {
		t.Fatal(err)
	}
	input.Write(request)
	args := []string{filepath.Join(dir, "metadata-only.json"), "pagevault@pagevault.local"}
	if code := run(args, &input, &output, &errors, filepath.Join(dir, "pagevault-host")); code != 0 {
		t.Fatalf("%d: %s", code, errors.String())
	}
	if errors.Len() != 0 || output.Len() < 4 {
		t.Fatalf("unexpected protocol output: %q / %s", output.Bytes(), errors.String())
	}
	length := binary.NativeEndian.Uint32(output.Bytes()[:4])
	if int(length) != output.Len()-4 {
		t.Fatal("stdout contains non-protocol data")
	}
	var reply struct {
		ID   string `json:"id"`
		OK   bool   `json:"ok"`
		Data struct {
			DBPath string `json:"db_path"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes()[4:], &reply); err != nil {
		t.Fatal(err)
	}
	if reply.ID != "firefox-ping" || !reply.OK || reply.Data.DBPath != db {
		t.Fatal(reply)
	}
	if _, err := os.Stat(args[0]); !os.IsNotExist(err) {
		t.Fatal("native metadata was used as a file", err)
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

func TestPackagePreflightRejectsUnregisteredSQLitePayload(t *testing.T) {
	for _, name := range []string{"pagevault-host", "LICENSE", "THIRD_PARTY_NOTICES.md"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, "Library", "Application Support", "PageVault", "Installer", name)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			original := []byte("SQLite format 3\x00synthetic archive bytes")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			if got := run([]string{"package-preflight", "--home", home}, strings.NewReader(""), &out, &stderr, "host"); got == 0 {
				t.Fatal("preflight accepted archive in payload destination")
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatalf("archive changed: %v", err)
			}
		})
	}
}

func TestPackagePreflightFreshHome(t *testing.T) {
	home := t.TempDir()
	var out, stderr bytes.Buffer
	if got := run([]string{"package-preflight", "--home", home}, strings.NewReader(""), &out, &stderr, "host"); got != 0 {
		t.Fatalf("fresh home rejected: %s", stderr.String())
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("preflight wrote files: %v %v", entries, err)
	}
}
