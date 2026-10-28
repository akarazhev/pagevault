// Package install installs a per-user, per-browser Native Messaging host.
package install

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"pagevault.local/pagevault/internal/buildinfo"
)

// Options selects the browser and installation paths. Platform and Home are
// primarily useful for previews; cross-platform writes are not permitted.
// ExecutablePath defaults to the currently running executable.
type Options struct {
	Browser, ExtensionID, ExtensionManifest, DBPath, RuntimeDir, ManifestDir string
	Home, Platform, ExecutablePath                                           string
	Uninstall, DryRun                                                        bool
	PreserveRegisteredDB                                                     bool
}

func currentPlatform() string {
	switch runtime.GOOS {
	case "linux":
		return "Linux"
	case "darwin":
		return "Darwin"
	case "windows":
		return "Windows"
	default:
		return runtime.GOOS
	}
}

func absolute(path, home string) (string, error) {
	if path == "~" {
		path = home
	} else if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		path = filepath.Join(home, path[2:])
	}
	return filepath.Abs(path)
}

func extensionID(o Options) (string, error) {
	id := o.ExtensionID
	if id == "" && o.ExtensionManifest != "" {
		b, err := os.ReadFile(o.ExtensionManifest)
		if err != nil {
			return "", fmt.Errorf("read extension manifest: %w", err)
		}
		var manifest struct {
			Key string `json:"key"`
		}
		if err = json.Unmarshal(b, &manifest); err != nil {
			return "", fmt.Errorf("read extension manifest: %w", err)
		}
		key, err := base64.StdEncoding.Strict().DecodeString(manifest.Key)
		if err != nil || len(key) == 0 {
			return "", errors.New("manifest must contain a valid base64 public key; or pass --extension-id")
		}
		digest := sha256.Sum256(key)
		encoded := make([]byte, 32)
		for i, b := range digest[:16] {
			encoded[i*2] = 'a' + b>>4
			encoded[i*2+1] = 'a' + b&15
		}
		id = string(encoded)
	}
	if id == "" {
		id = buildinfo.ExtensionID
	}
	if len(id) != 32 {
		return "", errors.New("extension ID must contain 32 letters a..p")
	}
	for _, c := range id {
		if c < 'a' || c > 'p' {
			return "", errors.New("extension ID must contain 32 letters a..p")
		}
	}
	return id, nil
}

// Run returns a JSON-compatible installation plan and, unless DryRun is set,
// applies it. Uninstallation removes only this browser's registration; runtime
// files and all databases remain available for an upgrade or a running host.
func Run(o Options) (map[string]any, error) {
	linux := map[string]string{"chrome": "google-chrome", "chromium": "chromium", "edge": "microsoft-edge", "brave": "BraveSoftware/Brave-Browser"}
	darwin := map[string]string{"chrome": "Google/Chrome", "chromium": "Chromium", "edge": "Microsoft Edge", "brave": "BraveSoftware/Brave-Browser"}
	registry := map[string]string{"chrome": `Google\Chrome`, "chromium": `Chromium`, "edge": `Microsoft\Edge`, "brave": `BraveSoftware\Brave-Browser`}
	if _, ok := linux[o.Browser]; !ok {
		return nil, errors.New("unsupported browser; choose chrome, chromium, edge or brave")
	}
	system := o.Platform
	if system == "" {
		system = currentPlatform()
	}
	home := o.Home
	var err error
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	id, err := extensionID(o)
	if err != nil {
		return nil, err
	}
	var app, native string
	switch system {
	case "Linux":
		app = filepath.Join(home, ".local", "share", "pagevault")
		native = filepath.Join(home, ".config", filepath.FromSlash(linux[o.Browser]), "NativeMessagingHosts")
	case "Darwin":
		app = filepath.Join(home, "Library", "Application Support", "PageVault")
		native = filepath.Join(home, "Library", "Application Support", filepath.FromSlash(darwin[o.Browser]), "NativeMessagingHosts")
	case "Windows":
		app = filepath.Join(home, "AppData", "Local", "PageVault")
		native = filepath.Join(app, "NativeMessagingHosts")
	default:
		return nil, errors.New("supported operating systems: Linux, macOS, Windows")
	}
	runtimeDir := filepath.Join(app, "host")
	if o.RuntimeDir != "" {
		runtimeDir, err = absolute(o.RuntimeDir, home)
		if err != nil {
			return nil, err
		}
	}
	db := filepath.Join(app, "archive.sqlite3")
	if o.DBPath != "" {
		db, err = absolute(o.DBPath, home)
		if err != nil {
			return nil, err
		}
	}
	if o.ManifestDir != "" {
		native, err = absolute(o.ManifestDir, home)
		if err != nil {
			return nil, err
		}
	}
	if system == "Windows" {
		native = filepath.Join(native, o.Browser)
	}
	name := "pagevault-host"
	if system == "Windows" {
		name += ".exe"
	}
	executable := filepath.Join(runtimeDir, o.Browser, name)
	config := filepath.Join(runtimeDir, o.Browser, "pagevault.json")
	manifestPath := filepath.Join(native, buildinfo.HostName+".json")
	adopted := false
	if o.DBPath == "" && o.PreserveRegisteredDB && !o.Uninstall {
		var previousDB string
		previousDB, adopted, err = registeredDatabase(manifestPath)
		if err != nil {
			return nil, err
		}
		if adopted {
			db = previousDB
		}
	}
	if o.DBPath == "" && !adopted {
		if data, readErr := os.ReadFile(config); readErr == nil {
			var previous struct {
				SchemaVersion int    `json:"schema_version"`
				DBPath        string `json:"db_path"`
			}
			parseErr := json.Unmarshal(data, &previous)
			if parseErr == nil && previous.SchemaVersion == 1 && filepath.IsAbs(previous.DBPath) {
				db = filepath.Clean(previous.DBPath)
			} else if !o.Uninstall {
				return nil, fmt.Errorf("invalid existing config %s; pass --db explicitly to repair it", config)
			}
		} else if !errors.Is(readErr, os.ErrNotExist) && !o.Uninstall {
			return nil, fmt.Errorf("read existing config: %w", readErr)
		}
	}
	for _, p := range []string{executable, config, manifestPath} {
		collision, checkErr := samePath(p, db, system)
		if checkErr != nil {
			return nil, fmt.Errorf("check database destination: %w", checkErr)
		}
		if collision {
			return nil, errors.New("database path must differ from installed executable, config and native manifest")
		}
	}
	manifest := map[string]any{"name": buildinfo.HostName, "description": "PageVault local SQLite archive", "path": executable, "type": "stdio", "allowed_origins": []string{"chrome-extension://" + id + "/"}}
	plan := map[string]any{"system": system, "browser": o.Browser, "runtime_dir": runtimeDir, "db_path": db, "executable_path": executable, "config_path": config, "manifest_path": manifestPath, "manifest": manifest, "uninstall": o.Uninstall, "dry_run": o.DryRun}
	if system == "Windows" {
		plan["registry_key"] = `Software\` + registry[o.Browser] + `\NativeMessagingHosts\` + buildinfo.HostName
	}
	if o.DryRun {
		return plan, nil
	}
	if system != currentPlatform() {
		return nil, errors.New("cross-platform plans may only be used with --dry-run")
	}
	if o.Uninstall {
		if system == "Windows" {
			if err = updateRegistry(plan["registry_key"].(string), manifestPath, true); err != nil {
				return nil, err
			}
		}
		if err = os.Remove(manifestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("remove native manifest: %w", err)
		}
		return plan, nil
	}
	source := o.ExecutablePath
	if source == "" {
		source, err = os.Executable()
		if err != nil {
			return nil, err
		}
	}
	// Open the source before creating installation directories. In particular, a
	// missing release binary must not disturb an existing registration.
	in, err := os.Open(source)
	if err != nil {
		return nil, fmt.Errorf("open host executable: %w", err)
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("host executable source must be a regular file")
	}
	configData, err := json.MarshalIndent(map[string]any{"schema_version": 1, "db_path": db}, "", "  ")
	if err != nil {
		return nil, err
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	tx := []*replacement{}
	defer func() {
		for _, r := range tx {
			r.cleanup()
		}
	}()
	for _, file := range []struct {
		path   string
		reader io.Reader
		mode   os.FileMode
	}{{executable, in, 0700}, {config, strings.NewReader(string(configData) + "\n"), 0600}, {manifestPath, strings.NewReader(string(manifestData) + "\n"), 0600}} {
		r, e := stage(file.path, file.reader, file.mode)
		if e != nil {
			return nil, e
		}
		tx = append(tx, r)
	}
	rollback := func(cause error) error {
		for i := len(tx) - 1; i >= 0; i-- {
			if e := tx[i].rollback(); e != nil {
				cause = errors.Join(cause, fmt.Errorf("restore %s: %w", tx[i].target, e))
			}
		}
		return cause
	}
	// Executable replacement comes first. Windows refuses to replace an image
	// currently in use; in that case old config and registration stay untouched.
	for _, r := range tx {
		if err = r.commit(); err != nil {
			return nil, rollback(err)
		}
	}
	if system == "Windows" {
		if err = updateRegistry(plan["registry_key"].(string), manifestPath, false); err != nil {
			return nil, rollback(err)
		}
	}
	return plan, nil
}

// resolvedDestination resolves existing ancestors even when the destination's
// final directories do not exist yet. This check is read-only and precedes all
// staging, registration changes and uninstallation.
func resolvedDestination(path string) (string, error) {
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		missing = append(missing, filepath.Base(path))
		path = parent
	}
}

func samePath(a, b, platform string) (bool, error) {
	a, err := resolvedDestination(a)
	if err != nil {
		return false, err
	}
	b, err = resolvedDestination(b)
	if err != nil {
		return false, err
	}
	if a == b || (platform == "Windows" && strings.EqualFold(a, b)) {
		return true, nil
	}
	parentA, err := os.Stat(filepath.Dir(a))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	parentB, err := os.Stat(filepath.Dir(b))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !os.SameFile(parentA, parentB) {
		return false, nil
	}
	nameA, nameB := filepath.Base(a), filepath.Base(b)
	if nameA == nameB || (platform == "Windows" && strings.EqualFold(nameA, nameB)) {
		return true, nil
	}
	fileA, err := os.Lstat(a)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	fileB, err := os.Lstat(b)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !os.SameFile(fileA, fileB) {
		return false, nil
	}
	// On a case-insensitive (or normalization-insensitive) filesystem, two
	// spellings can name one directory entry. Separate hard links have two
	// actual entries and replacing one cannot remove the other's archive.
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

// A replacement is fully staged, including a rollback copy, before any live
// file changes. Same-directory rename keeps a failed write from truncating a
// registered executable or its adjacent configuration.
type replacement struct {
	target, temp, backup string
	committed            bool
	preserveBackup       bool
}

func tempCopy(target string, in io.Reader, mode os.FileMode) (path string, err error) {
	f, err := os.CreateTemp(filepath.Dir(target), ".pagevault-*")
	if err != nil {
		return "", err
	}
	path = f.Name()
	temporaryPath := path
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(temporaryPath)
		}
	}()
	if err = f.Chmod(mode); err != nil {
		return "", err
	}
	if _, err = io.Copy(f, in); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	return path, nil
}
func stage(target string, in io.Reader, mode os.FileMode) (*replacement, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return nil, err
	}
	r := &replacement{target: target}
	success := false
	defer func() {
		if !success {
			r.cleanup()
		}
	}()
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("refusing to replace non-regular installation file %s", target)
		}
		old, e := os.Open(target)
		if e != nil {
			return nil, e
		}
		r.backup, e = tempCopy(target, old, info.Mode().Perm())
		closeErr := old.Close()
		if e != nil {
			return nil, e
		}
		if closeErr != nil {
			return nil, closeErr
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var err error
	r.temp, err = tempCopy(target, in, mode)
	if err != nil {
		return nil, fmt.Errorf("stage %s: %w", target, err)
	}
	success = true
	return r, nil
}
func (r *replacement) commit() error {
	if err := os.Rename(r.temp, r.target); err != nil {
		return fmt.Errorf("replace %s (close running browser hosts before retrying): %w", r.target, err)
	}
	r.temp = ""
	r.committed = true
	return nil
}
func (r *replacement) rollback() error {
	if !r.committed {
		return nil
	}
	if r.backup != "" {
		if err := os.Rename(r.backup, r.target); err != nil {
			r.preserveBackup = true
			return fmt.Errorf("recovery backup retained at %s: %w", r.backup, err)
		}
		r.backup = ""
		return nil
	}
	err := os.Remove(r.target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
func (r *replacement) cleanup() {
	if r.temp != "" {
		_ = os.Remove(r.temp)
	}
	if r.backup != "" && !r.preserveBackup {
		_ = os.Remove(r.backup)
	}
}
