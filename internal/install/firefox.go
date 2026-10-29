package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/akarazhev/pagevault/internal/buildinfo"
)

var firefoxIDPattern = regexp.MustCompile(`^(\{[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\}|[a-zA-Z0-9._-]*@[a-zA-Z0-9._-]+)$`)

// ValidFirefoxExtensionID accepts Gecko's email-style and braced UUID add-on IDs.
func ValidFirefoxExtensionID(id string) bool {
	return len(id) <= 80 && firefoxIDPattern.MatchString(id)
}

func firefoxExtensionID(o Options) (string, error) {
	id := o.ExtensionID
	if id == "" && o.ExtensionManifest != "" {
		raw, err := os.ReadFile(o.ExtensionManifest)
		if err != nil {
			return "", fmt.Errorf("read Firefox extension manifest: %w", err)
		}
		var manifest struct {
			BrowserSpecificSettings struct {
				Gecko struct {
					ID string `json:"id"`
				} `json:"gecko"`
			} `json:"browser_specific_settings"`
		}
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return "", fmt.Errorf("read Firefox extension manifest: %w", err)
		}
		id = manifest.BrowserSpecificSettings.Gecko.ID
	} else if id == "" {
		id = buildinfo.FirefoxExtensionID
	}
	if !ValidFirefoxExtensionID(id) {
		return "", errors.New("Firefox extension ID must be an email-style ID or a braced UUID (up to 80 characters)")
	}
	return id, nil
}
