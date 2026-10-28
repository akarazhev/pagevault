package macpkg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/macho"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"pagevault.local/pagevault/internal/archive"
	"pagevault.local/pagevault/internal/buildinfo"
	"pagevault.local/pagevault/internal/install"
)

func TestBuildRequiresExplicitSigningMode(t *testing.T) {
	for _, opts := range []Options{{}, {ApplicationIdentity: "app"}, {InstallerIdentity: "installer"}, {Unsigned: true, ApplicationIdentity: "app"}, {Unsigned: true, InstallerIdentity: "installer"}} {
		if _, err := Build(context.Background(), opts); err == nil {
			t.Fatal("accepted ambiguous signing mode", opts)
		}
	}
}

func TestBuildRejectsSourceAndAliasDestinations(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"extension", "cmd", "internal", "docs", "scripts", "tests", ".git"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(filepath.Join(root, "extension"), alias); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{filepath.Join(root, "README.md"), filepath.Join(root, "extension", "nested", "out-unsigned.pkg"), filepath.Join(alias, "out-unsigned.pkg")} {
		if _, err := Build(context.Background(), Options{Root: root, Output: output, Unsigned: true}); err == nil {
			t.Fatal("accepted unsafe output", output)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("created unsafe output", output, err)
		}
	}
}

func TestPackageRejectsAliasesBelowProtectedDirectories(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "extension", "ui")
	if err := os.MkdirAll(inside, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "ui alias")
	if err := os.Symlink(inside, alias); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{filepath.Join(alias, "out-unsigned.pkg"), filepath.Join(alias, "not-created", "nested", "out-unsigned.pkg")} {
		if err := safeOutput(root, output, true); err == nil {
			t.Fatalf("accepted package output through nested source alias: %s", output)
		}
	}
	if err := safeOutput(root, filepath.Join(t.TempDir(), "new", "out-unsigned.pkg"), true); err != nil {
		t.Fatal("blocked ordinary output", err)
	}
}

func TestBuildFailurePreservesPreviousArtifact(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native Apple packaging tools")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "previous-unsigned.pkg")
	for _, p := range []string{output, output + ".sha256"} {
		if err := os.WriteFile(p, []byte("preserved"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, err = Build(context.Background(), Options{Root: root, Output: output, Unsigned: true, Go: filepath.Join(t.TempDir(), "missing-go")})
	if err == nil {
		t.Fatal("missing compiler accepted")
	}
	for _, p := range []string{output, output + ".sha256"} {
		got, e := os.ReadFile(p)
		if e != nil || string(got) != "preserved" {
			t.Fatal(p, string(got), e)
		}
	}
}

func TestRealPackageRegistersTemporaryHomeAndReadsThroughMCP(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("real pkgbuild/productbuild and Mach-O execution")
	}
	if os.Geteuid() == 0 {
		t.Skip("user-home Installer scripts must run as a normal user")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "PageVault-test-unsigned.pkg")
	artifact, err := Build(ctx, Options{Root: root, Output: output, Unsigned: true, Go: filepath.Join(runtime.GOROOT(), "bin", "go")})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if artifact.Path != output || artifact.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal(artifact)
	}
	checksum, err := os.ReadFile(output + ".sha256")
	if err != nil || string(checksum) != artifact.SHA256+"  "+filepath.Base(output)+"\n" {
		t.Fatal(string(checksum), err)
	}
	expanded := filepath.Join(t.TempDir(), "expanded")
	command(t, ctx, "/usr/sbin/pkgutil", "--expand-full", output, expanded)
	dist, err := os.ReadFile(filepath.Join(expanded, "Distribution"))
	if err != nil {
		t.Fatal(err)
	}
	var distribution struct {
		Domains struct {
			User     string `xml:"enable_currentUserHome,attr"`
			System   string `xml:"enable_localSystem,attr"`
			Anywhere string `xml:"enable_anywhere,attr"`
		} `xml:"domains"`
		Choices []struct {
			ID       string `xml:"id,attr"`
			Selected string `xml:"start_selected,attr"`
		} `xml:"choice"`
		References []struct {
			ID        string `xml:"id,attr"`
			URL       string `xml:",chardata"`
			MustClose *struct {
				Apps []struct {
					ID string `xml:"id,attr"`
				} `xml:"app"`
			} `xml:"must-close"`
		} `xml:"pkg-ref"`
	}
	if err := xml.Unmarshal(dist, &distribution); err != nil {
		t.Fatal(err)
	}
	if distribution.Domains.User != "true" || distribution.Domains.System != "false" || distribution.Domains.Anywhere != "false" {
		t.Fatal("not restricted to current user's home", string(dist))
	}
	if !bytes.Contains(dist, []byte(`min="13.0"`)) {
		t.Fatal("missing minimum macOS version")
	}
	urls := map[string]int{}
	closures := map[string]string{}
	for _, ref := range distribution.References {
		if strings.TrimSpace(ref.URL) != "" {
			urls[ref.ID]++
		}
		if ref.MustClose != nil && len(ref.MustClose.Apps) == 1 {
			closures[ref.ID] = ref.MustClose.Apps[0].ID
		}
	}
	for _, id := range []string{"core", "chrome", "chromium", "edge", "brave"} {
		if urls["com.pagevault.host."+id] != 1 {
			t.Fatalf("ambiguous package URL for %s: %d", id, urls["com.pagevault.host."+id])
		}
	}
	for _, b := range browsers {
		if closures["com.pagevault.host."+b.ID] != b.BundleID {
			t.Fatal("missing browser close metadata", b.ID)
		}
	}
	choices := map[string]string{}
	for _, c := range distribution.Choices {
		choices[c.ID] = c.Selected
	}
	if choices["chrome"] != "true" {
		t.Fatal("Chrome not selected by default", choices)
	}
	for _, b := range []string{"chromium", "edge", "brave"} {
		if choices[b] != "false" {
			t.Fatal("optional browser not opt-in", b)
		}
	}
	payload := filepath.Join(expanded, "core.pkg", "Payload")
	hostRel := filepath.Join("Library", "Application Support", "PageVault", "Installer", "pagevault-host")
	fat, err := macho.OpenFat(filepath.Join(payload, hostRel))
	if err != nil {
		t.Fatal(err)
	}
	defer fat.Close()
	cpus := map[macho.Cpu]bool{}
	for _, a := range fat.Arches {
		cpus[a.Cpu] = true
	}
	if len(cpus) != 2 || !cpus[macho.CpuAmd64] || !cpus[macho.CpuArm64] {
		t.Fatal(cpus)
	}
	command(t, ctx, "/usr/bin/codesign", "--verify", "--strict", filepath.Join(payload, hostRel))
	home := filepath.Join(t.TempDir(), "Пользователь с пробелами")
	if err := os.CopyFS(home, os.DirFS(payload)); err != nil {
		t.Fatal(err)
	}
	for _, browser := range []string{"chrome", "chromium", "edge", "brave"} {
		script := filepath.Join(expanded, browser+".pkg", "Scripts", "postinstall")
		command(t, ctx, "/bin/sh", script, output, "/", home)
	}
	p, err := install.Run(install.Options{Browser: "chrome", Home: home, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	installed := p["executable_path"].(string)
	if got := command(t, ctx, installed, "version"); !strings.Contains(got, buildinfo.Version) {
		t.Fatal(got)
	}
	// A custom active registration must win over the freshly created default config.
	customDB := filepath.Join(t.TempDir(), "Активная база.sqlite3")
	customRuntime := filepath.Join(home, "старый runtime")
	_, err = install.Run(install.Options{Browser: "chrome", Home: home, RuntimeDir: customRuntime, DBPath: customDB, ExecutablePath: installed})
	if err != nil {
		t.Fatal(err)
	}
	w, err := archive.Open(customDB)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Save(map[string]any{"schema_version": 1, "url": "https://package-test.invalid/", "title": "PKG fixture", "captured_at": "2026-10-28T12:00:00Z", "blocks": []any{map[string]any{"type": "paragraph", "text": "before package upgrade"}}})
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	before, err := os.ReadFile(customDB)
	if err != nil {
		t.Fatal(err)
	}
	command(t, ctx, "/bin/sh", filepath.Join(expanded, "chrome.pkg", "Scripts", "postinstall"), output, "/", home)
	after, err := os.ReadFile(customDB)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("upgrade changed archive", err)
	}
	raw, err = os.ReadFile(p["config_path"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		DB string `json:"db_path"`
	}
	if err := json.Unmarshal(raw, &config); err != nil || config.DB != customDB {
		t.Fatal("active DB not retained", config, err)
	}
	// Capture through the actual copied native executable, then read through MCP.
	capture := map[string]any{"id": "pkg-capture", "type": "capture.save", "capture": map[string]any{"schema_version": 1, "url": "https://package-test.invalid/", "title": "PKG fixture", "captured_at": "2026-10-28T12:01:00Z", "blocks": []any{map[string]any{"type": "paragraph", "text": "native to SQLite to MCP 😀"}}}}
	body, err := json.Marshal(capture)
	if err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, 4)
	binary.NativeEndian.PutUint32(frame, uint32(len(body)))
	frame = append(frame, body...)
	cmd := exec.CommandContext(ctx, installed, "serve")
	cmd.Stdin = bytes.NewReader(frame)
	reply, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		OK bool `json:"ok"`
	}
	if len(reply) < 4 {
		t.Fatal("no native reply")
	}
	if err := json.Unmarshal(reply[4:], &saved); err != nil || !saved.OK {
		t.Fatal(string(reply[4:]), err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "installed-pkg-test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, installed, "mcp")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_pages", Arguments: map[string]any{"query": "native to SQLite"}})
	if err != nil || result.IsError {
		session.Close()
		t.Fatal(result, err)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil || !bytes.Contains(data, []byte(`"total":1`)) {
		session.Close()
		t.Fatal(string(data), err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	// Explicit target-home failure must never fall back to the actual user's home.
	bad := exec.CommandContext(ctx, "/bin/sh", filepath.Join(expanded, "chrome.pkg", "Scripts", "postinstall"), output, "/", filepath.Join(t.TempDir(), "missing"))
	if err := bad.Run(); err == nil {
		t.Fatal("accepted missing target payload")
	}
}

func command(t *testing.T, ctx context.Context, name string, args ...string) string {
	t.Helper()
	got, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", name, err, got)
	}
	return string(got)
}
