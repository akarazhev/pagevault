package mcpserver

import (
	"context"
	"fmt"

	"pagevault.local/pagevault/internal/archive"
)

type handler struct{ store *archive.Store }

type SearchInput struct {
	Query  string `json:"query,omitempty" jsonschema:"Case-insensitive text to find in saved title, URL or blocks; omitted means all pages."`
	Domain string `json:"domain,omitempty" jsonschema:"Exact hostname filter, for example example.org; no scheme or path."`
	Limit  *int64 `json:"limit,omitempty" jsonschema:"Maximum items, 1 to 100; default 20. Byte limits can return fewer."`
	Offset *int64 `json:"offset,omitempty" jsonschema:"Nonnegative item offset; default 0. Use next_offset for continuation."`
}

type VersionsInput struct {
	PageID int64  `json:"page_id" jsonschema:"Positive page ID from search_pages."`
	Limit  *int64 `json:"limit,omitempty" jsonschema:"Maximum items, 1 to 100; default 20. Byte limits can return fewer."`
	Offset *int64 `json:"offset,omitempty" jsonschema:"Nonnegative item offset; default 0. Use next_offset for continuation."`
}

type SitesInput struct {
	Limit  *int64 `json:"limit,omitempty" jsonschema:"Maximum items, 1 to 100; default 20. Byte limits can return fewer."`
	Offset *int64 `json:"offset,omitempty" jsonschema:"Nonnegative item offset; default 0. Use next_offset for continuation."`
}

type PageInput struct {
	PageID       int64  `json:"page_id" jsonschema:"Positive page ID from search_pages."`
	Offset       *int64 `json:"offset,omitempty" jsonschema:"Nonnegative Unicode code point offset; default 0. Follow next_offset."`
	MaxChars     *int64 `json:"max_chars,omitempty" jsonschema:"Maximum Unicode code points, 1 to 16000; default 8000. Byte limits can shorten the chunk."`
	DocumentHash string `json:"document_hash,omitempty" jsonschema:"SHA-256 from the previous chunk; required when offset is greater than 0. Restart from 0 if the document changed."`
}

type VersionInput struct {
	CaptureID    int64  `json:"capture_id" jsonschema:"Positive capture ID from list_versions."`
	Offset       *int64 `json:"offset,omitempty" jsonschema:"Nonnegative Unicode code point offset; default 0. Follow next_offset."`
	MaxChars     *int64 `json:"max_chars,omitempty" jsonschema:"Maximum Unicode code points, 1 to 16000; default 8000. Byte limits can shorten the chunk."`
	DocumentHash string `json:"document_hash,omitempty" jsonschema:"Optional SHA-256 from a previous chunk to verify the document."`
}

func number(value *int64, fallback, min, max int64, name string) (int64, error) {
	n := fallback
	if value != nil {
		n = *value
	}
	return archive.Integer(n, name, min, max)
}

func pagination(limit, offset *int64) (int64, int64, error) {
	l, err := number(limit, 20, 1, 100, "limit")
	if err != nil {
		return 0, 0, err
	}
	o, err := number(offset, 0, 0, -1, "offset")
	return l, o, err
}

func (h handler) searchPages(ctx context.Context, in SearchInput) (ListResult, error) {
	l, o, err := pagination(in.Limit, in.Offset)
	if err != nil {
		return ListResult{}, err
	}
	data, err := h.store.ListPagesContext(ctx, in.Query, in.Domain, l, o)
	if err != nil {
		return ListResult{}, err
	}
	return boundedList(data, "items", l, o)
}

func (h handler) listSites(ctx context.Context, in SitesInput) (ListResult, error) {
	l, o, err := pagination(in.Limit, in.Offset)
	if err != nil {
		return ListResult{}, err
	}
	data, err := h.store.ListSitesContext(ctx, l, o)
	if err != nil {
		return ListResult{}, err
	}
	return boundedList(data, "items", l, o)
}

func (h handler) listVersions(ctx context.Context, in VersionsInput) (ListResult, error) {
	l, o, err := pagination(in.Limit, in.Offset)
	if err != nil {
		return ListResult{}, err
	}
	data, err := h.store.GetPageContext(ctx, in.PageID, l, o)
	if err != nil {
		return ListResult{}, err
	}
	return boundedList(data, "captures", l, o)
}

func chunkParams(offset, maxChars *int64) (int64, int64, error) {
	o, err := number(offset, 0, 0, -1, "offset")
	if err != nil {
		return 0, 0, err
	}
	n, err := number(maxChars, 8000, 1, 16000, "max_chars")
	return o, n, err
}

func (h handler) getPage(ctx context.Context, in PageInput) (DocumentChunk, error) {
	o, n, err := chunkParams(in.Offset, in.MaxChars)
	if err != nil {
		return DocumentChunk{}, err
	}
	if o > 0 && in.DocumentHash == "" {
		return DocumentChunk{}, fmt.Errorf("document_hash is required for get_page continuation; restart with offset 0")
	}
	doc, err := h.store.ExportContext(ctx, in.PageID)
	if err != nil {
		return DocumentChunk{}, err
	}
	raw, err := archive.Canonical(doc)
	if err != nil {
		return DocumentChunk{}, err
	}
	return chunkDocument(raw, DocumentChunk{ArchiveKind: "observed_blocks", PageID: in.PageID}, o, n, in.DocumentHash)
}

func (h handler) getVersion(ctx context.Context, in VersionInput) (DocumentChunk, error) {
	o, n, err := chunkParams(in.Offset, in.MaxChars)
	if err != nil {
		return DocumentChunk{}, err
	}
	raw, err := h.store.CaptureJSONContext(ctx, in.CaptureID)
	if err != nil {
		return DocumentChunk{}, err
	}
	v, err := archive.Decode(raw)
	if err != nil {
		return DocumentChunk{}, err
	}
	doc, ok := v.(map[string]any)
	if !ok {
		return DocumentChunk{}, fmt.Errorf("stored capture must be a JSON object")
	}
	delete(doc, "html")
	raw, err = archive.Canonical(doc)
	if err != nil {
		return DocumentChunk{}, err
	}
	return chunkDocument(raw, DocumentChunk{ArchiveKind: "capture", CaptureID: in.CaptureID}, o, n, in.DocumentHash)
}
