package macpkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pagevault.local/pagevault/internal/buildinfo"
)

var browsers = []struct{ ID, Name, BundleID string }{
	{"chrome", "Google Chrome", "com.google.Chrome"},
	{"chromium", "Chromium", "org.chromium.Chromium"},
	{"edge", "Microsoft Edge", "com.microsoft.edgemac"},
	{"brave", "Brave", "com.brave.Browser"},
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
		fmt.Fprintf(&b, "  <choice id=\"%s\" start_selected=\"%t\" title=\"%s\" description=\"Подключить PageVault к %s\"><pkg-ref id=\"com.pagevault.host.%s\"/></choice>\n", v.ID, v.ID == "chrome", v.Name, v.Name, v.ID)
	}
	fmt.Fprintf(&b, "  <pkg-ref id=\"com.pagevault.host.core\" version=\"%s\">core.pkg</pkg-ref>\n", buildinfo.Version)
	for _, v := range browsers {
		fmt.Fprintf(&b, "  <pkg-ref id=\"com.pagevault.host.%s\" version=\"%s\">%s.pkg<must-close><app id=\"%s\"/></must-close></pkg-ref>\n", v.ID, buildinfo.Version, v.ID, v.BundleID)
	}
	b.WriteString("</installer-gui-script>\n")
	return []byte(b.String())
}

func writeResources(dir string, unsigned bool) error {
	note := ""
	if unsigned {
		note = "<p><strong>Тестовый пакет без подписи Developer ID.</strong> Для публичного распространения требуется подписанный и нотарифицированный выпуск.</p>"
	}
	pages := map[string]string{
		"welcome.html":    fmt.Sprintf(`<html lang="ru"><meta charset="utf-8"><body><h1>PageVault %s</h1>%s<p>Локальный помощник сохраняет выбранные страницы в SQLite и даёт агенту доступ к архиву через MCP.</p><p>Установка выполняется для текущего пользователя. На следующем шаге выберите используемые браузеры; Chrome включён по умолчанию. Поддерживаются macOS 13 и новее, Intel и Apple Silicon.</p><p>При обновлении сохраняется путь действующего архива. Установщик попросит закрыть выбранные браузеры; после обновления переподключите MCP-клиент.</p></body></html>`, buildinfo.Version, note),
		"conclusion.html": `<html lang="ru"><meta charset="utf-8"><body><h1>Помощник установлен</h1><p>Откройте браузер и настройки расширения PageVault, затем нажмите «Проверить подключение». Там будет показан фактический путь базы.</p><p>Включите запись для нужного сайта и сохраните страницу. Браузер сам запускает помощника; отдельную службу запускать не нужно.</p><p>Если подключаете Codex или Claude Code, выберите установленный pagevault-host из Library/Application Support/PageVault/host/ваш-браузер и команду mcp. Файл pagevault.json рядом с ним уже содержит путь архива.</p></body></html>`,
	}
	for name, page := range pages {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(page), 0644); err != nil {
			return err
		}
	}
	return nil
}
