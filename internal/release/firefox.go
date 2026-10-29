package release

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/akarazhev/pagevault/internal/buildinfo"
)

// Both packages use identical source assets; only the generated manifest differs.
func firefoxEntries(entries []entry) ([]entry, error) {
	var result []entry
	for _, source := range entries {
		if !strings.HasPrefix(source.name, "extension/") {
			continue
		}
		item := source
		item.name = "extension-firefox/" + strings.TrimPrefix(source.name, "extension/")
		if source.name == "extension/manifest.json" {
			raw, err := os.ReadFile(source.path)
			if err != nil {
				return nil, err
			}
			var manifest map[string]any
			if err := json.Unmarshal(raw, &manifest); err != nil {
				return nil, fmt.Errorf("Firefox manifest: %w", err)
			}
			delete(manifest, "key")
			delete(manifest, "minimum_chrome_version")
			manifest["background"] = map[string]any{"scripts": []string{"background.js"}, "type": "module"}
			manifest["browser_specific_settings"] = map[string]any{"gecko": map[string]any{
				"id": buildinfo.FirefoxExtensionID, "strict_min_version": "140.0",
				// Native Messaging transfers the selected pages to the local helper.
				"data_collection_permissions": map[string]any{"required": []string{"browsingActivity", "websiteContent"}},
			}}
			item.data, err = json.MarshalIndent(manifest, "", "  ")
			if err != nil {
				return nil, err
			}
			item.data = append(item.data, '\n')
			item.path = ""
		}
		result = append(result, item)
	}
	return result, nil
}
