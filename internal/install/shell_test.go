package install

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellInstallerMakesHostExecutableAndForwardsArguments(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("shell installer supports macOS and Linux")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("release binaries support amd64 and arm64")
	}
	projectRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "Проект с пробелами & symbols")
	for _, name := range []string{"install.sh", "extension/manifest.json"} {
		data, err := os.ReadFile(filepath.Join(projectRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(root, name), data)
	}
	mustWrite(t, filepath.Join(root, "extension-firefox", "manifest.json"), []byte(`{"browser_specific_settings":{"gecko":{"id":"pagevault@pagevault.local"}}}`))
	binary := filepath.Join(root, "bin", runtime.GOOS+"-"+runtime.GOARCH, "pagevault-host")
	if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		t.Fatal(err)
	}
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-trimpath", "-buildvcs=false", "-o", binary, "./cmd/pagevault")
	build.Dir = projectRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build host: %v\n%s", err, output)
	}
	if err := os.Chmod(binary, 0600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, "Данные & записи", "manual test.sqlite3")
	runtimeDir := filepath.Join(root, "runtime")
	manifestDir := filepath.Join(root, "registration")
	for _, browser := range []string{"chrome", "firefox"} {
		command := exec.Command("bash", filepath.Join(root, "install.sh"), browser, "--db", db, "--runtime-dir", runtimeDir, "--manifest-dir", manifestDir, "--home", filepath.Join(root, "home"), "--dry-run")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("shell installer: %v\n%s", err, output)
		}
		var result map[string]any
		if err := json.NewDecoder(strings.NewReader(string(output))).Decode(&result); err != nil {
			t.Fatalf("decode install plan: %v\n%s", err, output)
		}
		if result["browser"] != browser || result["db_path"] != db || result["dry_run"] != true {
			t.Fatalf("installer changed forwarded arguments: %v", result)
		}
	}
	info, err := os.Stat(binary)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0100 == 0 {
		t.Fatal("host was not made executable")
	}
	for _, path := range []string{db, runtimeDir, manifestDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("dry-run created %s or failed to inspect it: %v", path, err)
		}
	}
}
