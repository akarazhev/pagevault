package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"pagevault.local/pagevault/internal/buildinfo"
)

// registeredDatabase discovers the active archive without executing the old
// host. The destination runtime may differ from the currently registered one.
func registeredDatabase(manifestPath string) (string, bool, error) {
	repair := func(err error) (string, bool, error) {
		return "", false, fmt.Errorf("preserve registered archive from %s: %w; pass --db explicitly to repair", manifestPath, err)
	}
	raw, err := os.ReadFile(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return repair(err)
	}
	var manifest struct{ Name, Type, Path string }
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return repair(err)
	}
	if manifest.Name != buildinfo.HostName || manifest.Type != "stdio" || !filepath.IsAbs(manifest.Path) {
		return repair(errors.New("existing native manifest must identify PageVault stdio with an absolute executable path"))
	}
	configPath := filepath.Join(filepath.Dir(manifest.Path), "pagevault.json")
	raw, err = os.ReadFile(configPath)
	if err != nil {
		return repair(fmt.Errorf("read %s: %w", configPath, err))
	}
	var config struct {
		SchemaVersion int    `json:"schema_version"`
		DBPath        string `json:"db_path"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return repair(err)
	}
	if config.SchemaVersion != 1 || !filepath.IsAbs(config.DBPath) {
		return repair(fmt.Errorf("%s must contain schema_version 1 and an absolute db_path", configPath))
	}
	return filepath.Clean(config.DBPath), true, nil
}
