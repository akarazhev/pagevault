package release

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/akarazhev/pagevault/internal/buildinfo"
)

func TestFirefoxPackageUsesSharedAssetsAndGeckoManifest(t *testing.T) {
	root := extensionProject(t)
	put(t, root, "extension/manifest.json", fmt.Sprintf(`{"manifest_version":3,"version":%q,"key":"preserve-chromium-key","minimum_chrome_version":"116","background":{"service_worker":"background.js","type":"module"},"permissions":["storage","nativeMessaging"],"optional_host_permissions":["https://*/*","http://*/*"]}`, buildinfo.Version))
	put(t, root, "extension/background.js", "shared background")
	put(t, root, "extension/content.js", "shared capture")
	put(t, root, "extension/lib/outbox.js", "shared queue")
	put(t, root, "extension/private.sqlite3", "PRIVATE")
	original, err := os.ReadFile(filepath.Join(root, "extension", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, extensionOnly := range []bool{true, false} {
		output := filepath.Join(root, "dist", map[bool]string{true: "firefox.zip", false: "full.zip"}[extensionOnly])
		if err := Run(context.Background(), Options{Root: root, Output: output, Browser: "firefox", ExtensionOnly: extensionOnly, SourceOnly: !extensionOnly}); err != nil {
			t.Fatal(err)
		}
		files := archive(t, output)
		prefix := ""
		if !extensionOnly {
			prefix = "pagevault/extension-firefox/"
		}
		var manifest map[string]any
		if err := json.Unmarshal([]byte(content(t, files[prefix+"manifest.json"])), &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest["key"] != nil || manifest["minimum_chrome_version"] != nil {
			t.Fatal(manifest)
		}
		bg := manifest["background"].(map[string]any)
		if bg["service_worker"] != nil || bg["type"] != "module" || bg["scripts"].([]any)[0] != "background.js" {
			t.Fatal(bg)
		}
		gecko := manifest["browser_specific_settings"].(map[string]any)["gecko"].(map[string]any)
		if gecko["id"] != "pagevault@pagevault.local" || gecko["strict_min_version"] != "140.0" {
			t.Fatal(gecko)
		}
		disclosure := gecko["data_collection_permissions"].(map[string]any)["required"].([]any)
		if len(disclosure) != 2 || disclosure[0] != "browsingActivity" || disclosure[1] != "websiteContent" {
			t.Fatal(disclosure)
		}
		for name, want := range map[string]string{"background.js": "shared background", "content.js": "shared capture", "lib/outbox.js": "shared queue"} {
			if got := content(t, files[prefix+name]); got != want {
				t.Fatalf("%s: %q", name, got)
			}
		}
		if files[prefix+"private.sqlite3"] != nil {
			t.Fatal("private capture included")
		}
		if !extensionOnly && content(t, files["pagevault/extension/manifest.json"]) != string(original) {
			t.Fatal("Chromium manifest changed")
		}
		first, _ := os.ReadFile(output)
		if err := Run(context.Background(), Options{Root: root, Output: output, Browser: "firefox", ExtensionOnly: extensionOnly, SourceOnly: !extensionOnly}); err != nil {
			t.Fatal(err)
		}
		second, _ := os.ReadFile(output)
		if string(first) != string(second) {
			t.Fatal("non-deterministic Firefox package")
		}
	}
	unchanged, err := os.ReadFile(filepath.Join(root, "extension", "manifest.json"))
	if err != nil || string(unchanged) != string(original) {
		t.Fatal("source manifest modified", err)
	}
}

func TestUnsupportedPackageBrowserIsRejected(t *testing.T) {
	if err := Run(context.Background(), Options{Browser: "unknown"}); err == nil {
		t.Fatal("accepted unknown browser")
	}
}
