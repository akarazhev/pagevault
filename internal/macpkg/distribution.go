package macpkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akarazhev/pagevault/internal/buildinfo"
)

var browsers = []struct{ ID, Name, BundleID string }{
	{"chrome", "Google Chrome", "com.google.Chrome"},
	{"chromium", "Chromium", "org.chromium.Chromium"},
	{"edge", "Microsoft Edge", "com.microsoft.edgemac"},
	{"brave", "Brave", "com.brave.Browser"},
	{"firefox", "Mozilla Firefox", "org.mozilla.firefox"},
}

func distributionXML() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<installer-gui-script minSpecVersion="2">
  <title>PageVault %s</title>
  <welcome file="welcome.html" mime-type="text/html"/>
  <conclusion file="conclusion.html" mime-type="text/html"/>
  <options customize="always" require-scripts="false" hostArchitectures="x86_64,arm64"/>
  <domains enable_currentUserHome="true" enable_localSystem="false" enable_anywhere="false"/>
  <volume-check><allowed-os-versions><os-version min="13.0"/></allowed-os-versions></volume-check>
  <choices-outline><line choice="core"/>
`, buildinfo.Version)
	for _, v := range browsers {
		fmt.Fprintf(&b, "    <line choice=\"%s\"/>\n", v.ID)
	}
	b.WriteString(`  </choices-outline>
  <choice id="core" visible="false" start_selected="true" title="PageVault"><pkg-ref id="com.pagevault.host.core"/></choice>
`)
	for _, v := range browsers {
		fmt.Fprintf(&b, "  <choice id=\"%s\" start_selected=\"%t\" title=\"%s\" description=\"Connect PageVault to %s\"><pkg-ref id=\"com.pagevault.host.%s\"/></choice>\n", v.ID, v.ID == "chrome", v.Name, v.Name, v.ID)
	}
	fmt.Fprintf(&b, "  <pkg-ref id=\"com.pagevault.host.core\" version=\"%s\">core.pkg</pkg-ref>\n", buildinfo.Version)
	for _, v := range browsers {
		fmt.Fprintf(&b, "  <pkg-ref id=\"com.pagevault.host.%s\" version=\"%s\">%s.pkg</pkg-ref>\n", v.ID, buildinfo.Version, v.ID)
		fmt.Fprintf(&b, "  <pkg-ref id=\"com.pagevault.host.%s\"><must-close><app id=\"%s\"/></must-close></pkg-ref>\n", v.ID, v.BundleID)
	}
	b.WriteString("</installer-gui-script>\n")
	return []byte(b.String())
}

func writeResources(dir string, unsigned bool) error {
	note := ""
	if unsigned {
		note = "<p><strong>This package is unsigned and not notarized.</strong> macOS may require approval in Privacy &amp; Security. Download PageVault from the project GitHub Releases page.</p>"
	}
	pages := map[string]string{
		"welcome.html":    fmt.Sprintf(`<html lang="en"><meta charset="utf-8"><body><h1>PageVault %s</h1>%s<p>PageVault saves selected pages to a local SQLite archive and provides MCP access.</p><p>Install for the current user and select your browsers. Chrome is selected by default. Requires macOS 13 or later on Intel or Apple Silicon.</p><p>Upgrades preserve the active archive path. Close the selected browsers and reconnect your MCP client after upgrading.</p></body></html>`, buildinfo.Version, note),
		"conclusion.html": `<html lang="en"><meta charset="utf-8"><body><h1>Host installed</h1><p>Open the PageVault extension settings and select Check connection to see the archive path.</p><p>Enable capture for a site and save a page. The browser starts the host automatically.</p><p>For Codex or Claude Code, select pagevault-host under Library/Application Support/PageVault/host/your-browser and use the mcp command. The adjacent pagevault.json contains the archive path.</p></body></html>`,
	}
	for name, page := range pages {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(page), 0644); err != nil {
			return err
		}
	}
	return nil
}
