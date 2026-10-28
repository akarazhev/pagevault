package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"pagevault.local/pagevault/internal/buildinfo"
)

// CheckPackagePayload checks archive collisions without opening a database or writing files.
func CheckPackagePayload(home string) error {
	if !filepath.IsAbs(home) {
		return errors.New("package preflight requires an absolute home path")
	}
	app := filepath.Join(home, "Library", "Application Support", "PageVault")
	base := filepath.Join(app, "Installer")
	targets := []string{filepath.Join(base, "pagevault-host"), filepath.Join(base, "LICENSE"), filepath.Join(base, "THIRD_PARTY_NOTICES.md")}
	browsers := map[string]string{"chrome": "Google/Chrome", "chromium": "Chromium", "edge": "Microsoft Edge", "brave": "BraveSoftware/Brave-Browser"}
	for browser, dir := range browsers {
		manifest := filepath.Join(home, "Library", "Application Support", filepath.FromSlash(dir), "NativeMessagingHosts", buildinfo.HostName+".json")
		db, exists, err := registeredDatabase(manifest)
		if err != nil {
			return err
		}
		if exists {
			if err := checkPayloadDB(targets, db); err != nil {
				return err
			}
		}
		config := filepath.Join(app, "host", browser, "pagevault.json")
		raw, err := os.ReadFile(config)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", config, err)
		}
		var c struct {
			SchemaVersion int    `json:"schema_version"`
			DBPath        string `json:"db_path"`
		}
		if err := json.Unmarshal(raw, &c); err != nil || c.SchemaVersion != 1 || !filepath.IsAbs(c.DBPath) {
			return fmt.Errorf("invalid existing config %s; repair it before installing", config)
		}
		if err := checkPayloadDB(targets, c.DBPath); err != nil {
			return err
		}
	}
	for _, target := range targets {
		f, err := os.Open(target)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect payload destination %s: %w", target, err)
		}
		header := make([]byte, 16)
		n, readErr := io.ReadFull(f, header)
		closeErr := f.Close()
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n == 16 && string(header) == "SQLite format 3\x00" {
			return fmt.Errorf("payload destination %s contains a SQLite archive; move it before installing", target)
		}
	}
	return nil
}
func checkPayloadDB(targets []string, db string) error {
	for _, target := range targets {
		same, err := samePath(target, db, "Darwin")
		if err != nil {
			return err
		}
		if same {
			return fmt.Errorf("archive %s conflicts with installer payload %s; move the archive before installing", db, target)
		}
	}
	return nil
}
