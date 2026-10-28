package install

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/akarazhev/pagevault/internal/buildinfo"
)

func plan(t *testing.T, o Options) map[string]any {
	t.Helper()
	p, err := Run(o)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func object(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err = json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func manifest(p map[string]any) map[string]any { return p["manifest"].(map[string]any) }
func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
}
func assertBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, want) {
		t.Fatalf("%s contains %q, want %q", path, b, want)
	}
}

func TestDryRunAllPlatformBrowserPaths(t *testing.T) {
	profiles := map[string]map[string]string{"Linux": {"chrome": ".config/google-chrome", "chromium": ".config/chromium", "edge": ".config/microsoft-edge", "brave": ".config/BraveSoftware/Brave-Browser"}, "Darwin": {"chrome": "Library/Application Support/Google/Chrome", "chromium": "Library/Application Support/Chromium", "edge": "Library/Application Support/Microsoft Edge", "brave": "Library/Application Support/BraveSoftware/Brave-Browser"}}
	home := filepath.Join(t.TempDir(), "Пользователь %USER% & space!")
	for _, system := range []string{"Linux", "Darwin", "Windows"} {
		for _, browser := range []string{"chrome", "chromium", "edge", "brave"} {
			t.Run(system+"/"+browser, func(t *testing.T) {
				p := plan(t, Options{Browser: browser, Platform: system, Home: home, DryRun: true})
				origins := manifest(p)["allowed_origins"].([]string)
				if len(origins) != 1 || origins[0] != "chrome-extension://"+buildinfo.ExtensionID+"/" {
					t.Fatal(origins)
				}
				exe := p["executable_path"].(string)
				if !filepath.IsAbs(exe) || filepath.Base(filepath.Dir(exe)) != browser {
					t.Fatal(exe)
				}
				wantExe := "pagevault-host"
				if system == "Windows" {
					wantExe += ".exe"
					if !strings.Contains(p["registry_key"].(string), "NativeMessagingHosts\\"+buildinfo.HostName) {
						t.Fatal(p)
					}
				}
				if filepath.Base(exe) != wantExe || manifest(p)["path"] != exe || p["config_path"] != filepath.Join(filepath.Dir(exe), "pagevault.json") {
					t.Fatal(p)
				}
				if system != "Windows" {
					want := filepath.Join(home, filepath.FromSlash(profiles[system][browser]), "NativeMessagingHosts", buildinfo.HostName+".json")
					if p["manifest_path"] != want {
						t.Fatalf("%v != %s", p["manifest_path"], want)
					}
				} else if filepath.Base(filepath.Dir(p["manifest_path"].(string))) != browser {
					t.Fatal(p)
				}
			})
		}
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote destination: %v", err)
	}
}

func TestExplicitExtensionManifestAndID(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "manifest.json")
	key := []byte("public-key")
	raw, _ := json.Marshal(map[string]string{"key": base64.StdEncoding.EncodeToString(key)})
	mustWrite(t, path, raw)
	digest := sha256.Sum256(key)
	want := ""
	for _, b := range digest[:16] {
		want += string([]byte{'a' + b>>4, 'a' + b&15})
	}
	p := plan(t, Options{Browser: "chrome", Home: home, ExtensionManifest: path, DryRun: true})
	if manifest(p)["allowed_origins"].([]string)[0] != "chrome-extension://"+want+"/" {
		t.Fatal(p)
	}
	p = plan(t, Options{Browser: "chrome", Home: home, ExtensionManifest: filepath.Join(home, "missing"), ExtensionID: strings.Repeat("b", 32), DryRun: true})
	if manifest(p)["allowed_origins"].([]string)[0] != "chrome-extension://"+strings.Repeat("b", 32)+"/" {
		t.Fatal(p)
	}
	for _, bad := range []string{`{}`, `{"key":"!!!"}`, `{"key":12}`, `{"key":""}`, `not json`} {
		mustWrite(t, path, []byte(bad))
		if _, err := Run(Options{Browser: "chrome", Home: home, ExtensionManifest: path, DryRun: true}); err == nil {
			t.Fatalf("accepted manifest %s", bad)
		}
	}
}

func TestInvalidOptionsAndCrossPlatformWrite(t *testing.T) {
	for _, o := range []Options{{Browser: "firefox", DryRun: true}, {Browser: "chrome", Platform: "Plan9", DryRun: true}, {Browser: "chrome", ExtensionID: strings.Repeat("z", 32), DryRun: true}, {Browser: "../chrome", DryRun: true}} {
		if _, err := Run(o); err == nil {
			t.Fatalf("accepted %+v", o)
		}
	}
	other := "Windows"
	if runtime.GOOS == "windows" {
		other = "Linux"
	}
	home := filepath.Join(t.TempDir(), "absent")
	if _, err := Run(Options{Browser: "chrome", Home: home, Platform: other}); err == nil {
		t.Fatal("cross platform install permitted")
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("wrote cross platform destination")
	}
}

func TestInstallIndependentBrowsersUpgradeAndUninstallPreserveDatabase(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("real Linux registration")
	}
	home := filepath.Join(t.TempDir(), "Пользователь %USER% & space!")
	src := filepath.Join(t.TempDir(), "installer")
	mustWrite(t, src, []byte("first executable\x00bytes"))
	db := filepath.Join(home, "Архив %DB% & !.sqlite3")
	mustWrite(t, db, []byte("preserve database"))
	chromeOpts := Options{Browser: "chrome", Home: home, DBPath: db, ExecutablePath: src}
	chrome := plan(t, chromeOpts)
	exe := chrome["executable_path"].(string)
	cfg := chrome["config_path"].(string)
	assertBytes(t, exe, []byte("first executable\x00bytes"))
	if object(t, cfg)["db_path"] != db || object(t, cfg)["schema_version"] != float64(1) {
		t.Fatal(object(t, cfg))
	}
	if object(t, chrome["manifest_path"].(string))["path"] != exe {
		t.Fatal(chrome)
	}
	info, err := os.Stat(exe)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("executable mode: %v %v", info, err)
	}
	edgeOpts := Options{Browser: "edge", Home: home, DBPath: filepath.Join(home, "edge.sqlite3"), ExecutablePath: src, ExtensionID: strings.Repeat("b", 32)}
	edge := plan(t, edgeOpts)
	if edge["executable_path"] == exe || edge["config_path"] == cfg {
		t.Fatal("shared browser files")
	}
	mustWrite(t, src, []byte("upgraded executable"))
	plan(t, edgeOpts)
	assertBytes(t, exe, []byte("first executable\x00bytes"))
	assertBytes(t, edge["executable_path"].(string), []byte("upgraded executable"))
	assertBytes(t, db, []byte("preserve database"))
	plan(t, chromeOpts)
	assertBytes(t, exe, []byte("upgraded executable"))
	assertBytes(t, db, []byte("preserve database"))
	chromeOpts.Uninstall = true
	plan(t, chromeOpts)
	plan(t, chromeOpts)
	if _, err := os.Stat(chrome["manifest_path"].(string)); !os.IsNotExist(err) {
		t.Fatal("manifest retained")
	}
	assertBytes(t, db, []byte("preserve database"))
	assertBytes(t, edge["executable_path"].(string), []byte("upgraded executable"))
	if _, err := os.Stat(edge["manifest_path"].(string)); err != nil {
		t.Fatal(err)
	}
}

func TestFailedUpdateLeavesPreviousInstallUsable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux filesystem check")
	}
	home := t.TempDir()
	src := filepath.Join(home, "source")
	mustWrite(t, src, []byte("original"))
	o := Options{Browser: "chrome", Home: home, ExecutablePath: src}
	p := plan(t, o)
	oldConfig, _ := os.ReadFile(p["config_path"].(string))
	oldManifest, _ := os.ReadFile(p["manifest_path"].(string))
	o.ExecutablePath = filepath.Join(home, "missing")
	o.DBPath = filepath.Join(home, "new.sqlite3")
	if _, err := Run(o); err == nil {
		t.Fatal("missing source succeeded")
	}
	assertBytes(t, p["executable_path"].(string), []byte("original"))
	assertBytes(t, p["config_path"].(string), oldConfig)
	assertBytes(t, p["manifest_path"].(string), oldManifest)
	// Failure replacing the executable must precede any config/manifest change.
	o.ExecutablePath = src
	exe := p["executable_path"].(string)
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(exe, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(o); err == nil {
		t.Fatal("directory executable replacement succeeded")
	}
	assertBytes(t, p["config_path"].(string), oldConfig)
	assertBytes(t, p["manifest_path"].(string), oldManifest)
}

func TestUpgradeWithoutDBFlagRetainsPerBrowserConfiguration(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("real Linux installation")
	}
	home := t.TempDir()
	src := filepath.Join(home, "source")
	mustWrite(t, src, []byte("version one"))
	custom := filepath.Join(home, "данные", "custom.sqlite3")
	mustWrite(t, custom, []byte("archived content"))
	o := Options{Browser: "chrome", Home: home, ExecutablePath: src, DBPath: custom}
	p := plan(t, o)
	o.DBPath = ""
	o.DryRun = true
	preview := plan(t, o)
	if preview["db_path"] != custom {
		t.Fatalf("upgrade preview reset custom database: %v", preview["db_path"])
	}
	o.DryRun = false
	mustWrite(t, src, []byte("version two"))
	p = plan(t, o)
	if p["db_path"] != custom || object(t, p["config_path"].(string))["db_path"] != custom {
		t.Fatal("upgrade reset custom database")
	}
	assertBytes(t, custom, []byte("archived content"))
	edge := plan(t, Options{Browser: "edge", Home: home, ExecutablePath: src})
	if edge["db_path"] == custom {
		t.Fatal("another browser inherited chrome custom path")
	}
	// Invalid existing configuration must not silently select a different archive.
	mustWrite(t, p["config_path"].(string), []byte(`{"schema_version":1,"db_path":"relative.sqlite3"}`))
	if _, err := Run(o); err == nil {
		t.Fatal("invalid existing config silently replaced")
	}
	assertBytes(t, custom, []byte("archived content"))
	o.DBPath = custom
	plan(t, o)
	if object(t, p["config_path"].(string))["db_path"] != custom {
		t.Fatal("explicit database did not repair config")
	}
}

func TestRejectDatabasePathAliasingInstalledFiles(t *testing.T) {
	for _, key := range []string{"executable_path", "config_path", "manifest_path"} {
		o := Options{Browser: "chrome", Home: t.TempDir(), DryRun: true}
		p := plan(t, o)
		o.DBPath = p[key].(string)
		if _, err := Run(o); err == nil {
			t.Fatalf("accepted database colliding with %s", key)
		}
	}
}

type failedRead struct{}

func (failedRead) Read(p []byte) (int, error) { return 0, os.ErrPermission }

func TestFailedStagingRemovesTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "pagevault-host")
	mustWrite(t, target, []byte("previous working executable"))
	if _, err := stage(target, failedRead{}, 0700); err == nil {
		t.Fatal("failed read accepted")
	}
	assertBytes(t, target, []byte("previous working executable"))
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name() != "pagevault-host" {
		t.Fatalf("failed installation leaked staged files: %v", files)
	}
}

func TestFailedRollbackRetainsRecoveryBackupAfterCleanup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "pagevault-host")
	previous := []byte("previous working executable")
	mustWrite(t, target, previous)
	replacement, err := stage(target, strings.NewReader("new executable"), 0700)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.cleanup()
	backup := replacement.backup
	if err = replacement.commit(); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(target); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(target, "external-process-blocker"), []byte("busy"))
	err = replacement.rollback()
	if err == nil {
		t.Fatal("rollback unexpectedly replaced nonempty directory")
	}
	replacement.cleanup()
	assertBytes(t, backup, previous)
	if !strings.Contains(err.Error(), backup) {
		t.Fatalf("rollback error omits recovery path %s: %v", backup, err)
	}
}

func TestRejectDatabaseAliasesThroughSymlinkedParentsBeforeInstallOrUninstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require Windows developer mode")
	}
	for _, key := range []string{"executable_path", "config_path", "manifest_path"} {
		for _, uninstall := range []bool{false, true} {
			t.Run(key+"/uninstall="+map[bool]string{false: "false", true: "true"}[uninstall], func(t *testing.T) {
				home := t.TempDir()
				o := Options{Browser: "chrome", Home: home, DryRun: true}
				p := plan(t, o)
				destination := p[key].(string)
				mustWrite(t, destination, []byte("archive bytes must survive"))
				alias := filepath.Join(home, "alias")
				if err := os.Symlink(filepath.Dir(destination), alias); err != nil {
					t.Fatal(err)
				}
				source := filepath.Join(home, "source")
				mustWrite(t, source, []byte("new executable"))
				o.ExecutablePath = source
				o.DBPath = filepath.Join(alias, filepath.Base(destination))
				o.DryRun = false
				o.Uninstall = uninstall
				if _, err := Run(o); err == nil {
					t.Fatal("accepted aliased database destination")
				}
				assertBytes(t, destination, []byte("archive bytes must survive"))
				// The collision must fail before staging creates runtime or manifest siblings.
				files, err := os.ReadDir(filepath.Dir(destination))
				if err != nil {
					t.Fatal(err)
				}
				if len(files) != 1 {
					t.Fatalf("alias guard ran after filesystem writes: %v", files)
				}
			})
		}
	}
}

func TestRejectDatabaseAliasesThroughExistingAncestorWithMissingChildren(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require Windows developer mode")
	}
	home := t.TempDir()
	real := filepath.Join(home, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(home, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	o := Options{Browser: "chrome", Home: home, RuntimeDir: real, DBPath: filepath.Join(alias, "chrome", "pagevault.json"), DryRun: true}
	if _, err := Run(o); err == nil {
		t.Fatal("accepted ancestor alias with missing children")
	}
	files, err := os.ReadDir(real)
	if err != nil || len(files) != 0 {
		t.Fatalf("preview wrote through alias: %v %v", files, err)
	}
}

func TestCaseInsensitiveDestinationAliasPreservesDatabase(t *testing.T) {
	home := t.TempDir()
	o := Options{Browser: "chrome", Home: home, DryRun: true}
	p := plan(t, o)
	destination := p["manifest_path"].(string)
	mustWrite(t, destination, []byte("archive bytes must survive"))
	alias := filepath.Join(filepath.Dir(destination), strings.ToUpper(filepath.Base(destination)))
	if _, err := os.Stat(alias); os.IsNotExist(err) {
		t.Skip("filesystem is case sensitive")
	} else if err != nil {
		t.Fatal(err)
	}
	o.DBPath = alias
	o.DryRun = false
	o.Uninstall = true
	if _, err := Run(o); err == nil {
		t.Fatal("accepted case-insensitive database alias")
	}
	assertBytes(t, destination, []byte("archive bytes must survive"))
}

func TestSeparateHardLinkDoesNotCollideWithReplacementDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cross-platform hard-link permissions")
	}
	home := t.TempDir()
	o := Options{Browser: "chrome", Home: home, DryRun: true}
	p := plan(t, o)
	destination := p["manifest_path"].(string)
	archive := []byte("archive retained through separate hard link")
	mustWrite(t, destination, archive)
	database := filepath.Join(filepath.Dir(destination), "independent.sqlite3")
	if err := os.Link(destination, database); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(home, "source")
	mustWrite(t, source, []byte("native executable"))
	o.ExecutablePath = source
	o.DBPath = database
	o.DryRun = false
	plan(t, o)
	assertBytes(t, database, archive)
}
