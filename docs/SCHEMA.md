# Формат данных

## Снимок `schema_version: 1`

Снимок описывает доступное содержимое одной страницы в определённый момент. Пример условный:

```json
{
  "schema_version": 1,
  "url": "https://example.org/thread/42?page=2#reply",
  "title": "Обсуждение проекта",
  "lang": "ru",
  "captured_at": "2026-10-27T12:00:00.000Z",
  "reason": "mutation",
  "session_id": "local-browser-document-id",
  "source": {"kind": "dom", "adapter": "generic"},
  "metadata": {"description": "", "canonical_url": "", "site_data": {}},
  "blocks": [
    {"type": "heading", "level": 1, "text": "Обсуждение проекта"},
    {"type": "paragraph", "text": "Текст сообщения"},
    {"type": "code", "language": "python", "text": "def hello():\n    return 'hello'"},
    {"type": "table", "rows": [["Имя", "Статус"], ["А", "Готово"]]},
    {"type": "image", "src": "https://example.org/image.png", "alt": "Схема"}
  ],
  "warnings": [],
  "stats": {"block_count": 5, "text_chars": 100}
}
```

`html` добавляется, когда включено сохранение очищенного HTML. `warnings` содержит объекты `{code,message}`. `captured_at` — время RFC3339 с часовым поясом; браузер формирует UTC через `Date.toISOString()`. `reason` различает начальный снимок, изменения, контрольную проверку и ручной запуск.

## Блоки

| `type` | Основные поля |
|---|---|
| `heading` | `level`, `text` |
| `paragraph`, `text` | `text`, при необходимости `links` |
| `code` | `text` с пробелами/переводами строк, `language` при наличии |
| `list` | `ordered`, `items`; сложный элемент может иметь `text` и вложенные `blocks` |
| `table` | `rows`; сложные ячейки дополнительно представлены `cell_blocks[row][cell]` |
| `link` | `url`, `text` |
| `image` | `src`, `alt`, дополнительные подписи |
| `media` | `kind`, `src`, возможная `poster` |

`links` — список `{text,url}`. Относительные адреса преобразуются в абсолютные HTTP(S)-адреса. Вложенные структуры сохраняют порядок; неизвестные типы UI показывает как текст JSON.

## Идентичность и версии

Страница определяется точным URL, включая query и hash. Удаление этих частей для всех сайтов потеряло бы часть состояний SPA, поэтому универсальной канонизации нет. `metadata.canonical_url` сохраняется отдельно и не заменяет адрес просмотра.

Хэш снимка учитывает семантические поля `schema_version`, `url`, `title`, `lang`, `source`, `metadata`, `blocks`. Временные поля и HTML не создают отдельную версию. При повторении содержимого сохраняется уже существующий снимок; `pages.last_seen` обновляется.

## Накопленный экспорт

```json
{
  "schema_version": 1,
  "archive_kind": "observed_blocks",
  "page": {"id": 1, "url": "https://example.org/thread/42", "title": "Обсуждение"},
  "blocks": []
}
```

`blocks` — уникальные блоки в порядке первого наблюдения. Исчезновение блока из текущего DOM не удаляет его из этого набора. Точная структура и порядок одного состояния доступны в исходном снимке.

У кратких ответов API может быть `summary_truncated`: список полей, сокращённых ради ограничения размера Native Messaging. Для длинного URL краткий `url` пустой, а `url_display` предназначен только для отображения. Полные значения находятся в chunked экспорте и исходных снимках.
