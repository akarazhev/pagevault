package mcpserver

import (
	"encoding/json"
	"fmt"

	"pagevault.local/pagevault/internal/archive"
)

const maxDataBytes = 32 * 1024

type ListResult struct {
	Items      []map[string]any `json:"items"`
	Total      int64            `json:"total"`
	Limit      int64            `json:"limit"`
	Offset     int64            `json:"offset"`
	NextOffset *int64           `json:"next_offset"`
	Page       map[string]any   `json:"page,omitempty"`
}

func boundedList(data map[string]any, itemKey string, limit, offset int64) (ListResult, error) {
	r := ListResult{Items: data[itemKey].([]map[string]any), Total: data["total"].(int64), Limit: limit, Offset: offset}
	r.Page, _ = data["page"].(map[string]any)
	for {
		r.NextOffset = nil
		if len(r.Items) > 0 && offset < r.Total && int64(len(r.Items)) < r.Total-offset {
			n := offset + int64(len(r.Items))
			r.NextOffset = &n
		}
		raw, err := json.Marshal(r)
		if err != nil {
			return ListResult{}, err
		}
		if len(raw) <= maxDataBytes {
			return r, nil
		}
		if len(r.Items) <= 1 {
			return ListResult{}, fmt.Errorf("archive summary exceeds MCP response limit")
		}
		r.Items = r.Items[:len(r.Items)-1]
	}
}

type DocumentChunk struct {
	ArchiveKind  string `json:"archive_kind"`
	PageID       int64  `json:"page_id,omitempty"`
	CaptureID    int64  `json:"capture_id,omitempty"`
	Format       string `json:"format"`
	Text         string `json:"text"`
	Done         bool   `json:"done"`
	Offset       int64  `json:"offset"`
	NextOffset   *int64 `json:"next_offset"`
	TotalChars   int64  `json:"total_chars"`
	DocumentHash string `json:"document_hash"`
}

func chunkDocument(raw string, r DocumentChunk, offset, count int64, expectedHash string) (DocumentChunk, error) {
	r.Format = "json"
	r.DocumentHash = archive.Hash(raw)
	if expectedHash != "" && expectedHash != r.DocumentHash {
		return DocumentChunk{}, fmt.Errorf("document changed or document_hash does not match; restart with offset 0")
	}
	runes := []rune(raw)
	r.TotalChars = int64(len(runes))
	if offset > r.TotalChars {
		return DocumentChunk{}, fmt.Errorf("offset exceeds total_chars (%d)", r.TotalChars)
	}
	r.Offset = offset
	if count > r.TotalChars-offset {
		count = r.TotalChars - offset
	}
	setText := func(n int64) {
		r.Text = string(runes[offset : offset+n])
		r.Done = offset+n == r.TotalChars
		r.NextOffset = nil
		if offset+n < r.TotalChars {
			next := offset + n
			r.NextOffset = &next
		}
	}
	// Bound actual encoded JSON, including quoting/control characters, not just
	// source bytes or code points. Find the largest prefix that fits.
	low, high := int64(0), count
	for low < high {
		mid := low + (high-low+1)/2
		setText(mid)
		encoded, err := json.Marshal(r)
		if err != nil {
			return DocumentChunk{}, err
		}
		if len(encoded) <= maxDataBytes {
			low = mid
		} else {
			high = mid - 1
		}
	}
	setText(low)
	if low == 0 && count > 0 {
		return DocumentChunk{}, fmt.Errorf("document chunk exceeds MCP response limit")
	}
	return r, nil
}
