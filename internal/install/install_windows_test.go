//go:build windows

package install

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// This subprocess keeps the installed executable mapped until its stdin closes.
func TestInstallerBusyExecutableHelper(t *testing.T) {
	if os.Getenv("PAGEVAULT_INSTALL_BUSY_HELPER") != "1" {
		return
	}
	os.Stdout.WriteString("ready\n")
	io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func TestRunningWindowsExecutableUpdatePreservesRegistration(t *testing.T) {
	home := t.TempDir()
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	o := Options{Browser: "chrome", Home: home, DBPath: filepath.Join(home, "old.sqlite3"), ExecutablePath: source, DryRun: true}
	p := plan(t, o)
	binary, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	exe := p["executable_path"].(string)
	cfg := p["config_path"].(string)
	registration := p["manifest_path"].(string)
	oldConfig := []byte("{\"schema_version\":1,\"db_path\":\"old\"}")
	oldManifest := []byte("existing browser registration")
	mustWrite(t, exe, binary)
	mustWrite(t, cfg, oldConfig)
	mustWrite(t, registration, oldManifest)
	child := exec.Command(exe, "-test.run=^TestInstallerBusyExecutableHelper$")
	child.Env = append(os.Environ(), "PAGEVAULT_INSTALL_BUSY_HELPER=1")
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { stdin.Close(); child.Wait() }()
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("helper failed to become ready: %q %v", line, err)
	}
	o.DryRun = false
	o.DBPath = filepath.Join(home, "new.sqlite3")
	if _, err = Run(o); err == nil {
		t.Fatal("Windows replaced a running executable")
	}
	assertBytes(t, exe, binary)
	assertBytes(t, cfg, oldConfig)
	assertBytes(t, registration, oldManifest)
}
