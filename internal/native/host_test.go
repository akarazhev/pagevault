package native

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"pagevault.local/pagevault/internal/archive"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func frame(raw string) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.NativeEndian, uint32(len(raw)))
	b.WriteString(raw)
	return b.Bytes()
}
func replies(t *testing.T, b []byte) []map[string]any {
	t.Helper()
	r := bytes.NewReader(b)
	out := []map[string]any{}
	for r.Len() > 0 {
		var n uint32
		if e := binary.Read(r, binary.NativeEndian, &n); e != nil {
			t.Fatal(e)
		}
		if n >= MaxOutgoing {
			t.Fatal(n)
		}
		raw := make([]byte, n)
		if _, e := io.ReadFull(r, raw); e != nil {
			t.Fatal(e)
		}
		var v map[string]any
		if e := json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		out = append(out, v)
	}
	return out
}
func TestPersistentServeMalformedRecovery(t *testing.T) {
	var in, out bytes.Buffer
	for _, raw := range []string{`{"id":"p1","type":"ping"}`, `{`, `{"id":"s","type":"stats"}`, `{"id":"p2","type":"ping"}`} {
		in.Write(frame(raw))
	}
	if e := Serve(filepath.Join(t.TempDir(), "db"), &in, &out); e != nil {
		t.Fatal(e)
	}
	rs := replies(t, out.Bytes())
	if len(rs) != 4 || rs[1]["ok"] != false || rs[2]["ok"] != true {
		t.Fatal(rs)
	}
	a := rs[0]["data"].(map[string]any)
	b := rs[3]["data"].(map[string]any)
	for _, k := range []string{"host_id", "process_id", "go_version", "sqlite_version", "db_connections"} {
		if a[k] == nil || a[k] != b[k] {
			t.Fatal(k, a, b)
		}
	}
	if a["db_connections"] != float64(1) {
		t.Fatal(a)
	}
}
func TestInvalidUnicodeAndNumbersContinue(t *testing.T) {
	for _, raw := range []string{`{"id":"\ud800","type":"ping"}`, `{"id":"bad","type":"capture","capture":{"title":"\ud800"}}`, `{"id":"bad","type":"pages.get","page_id":true}`, `{"id":"bad","type":"pages.list","limit":1.5}`, `{"id":"bad","type":"pages.list","query":null}`, `{"id":"bad","type":"ping","x":NaN}`} {
		var out bytes.Buffer
		input := append(frame(raw), frame(`{"id":"next","type":"ping"}`)...)
		if e := Serve(filepath.Join(t.TempDir(), "db"), bytes.NewReader(input), &out); e != nil {
			t.Fatal(e)
		}
		rs := replies(t, out.Bytes())
		if rs[0]["ok"] != false || rs[1]["ok"] != true {
			t.Fatal(raw, rs)
		}
	}
}
func TestTruncatedAndOversizedFrames(t *testing.T) {
	for _, in := range [][]byte{{1, 0}, append([]byte{20, 0, 0, 0}, []byte("{}")...), {0, 0, 16, 2}} {
		var out bytes.Buffer
		if e := Serve(filepath.Join(t.TempDir(), "db"), bytes.NewReader(in), &out); e == nil {
			t.Fatal("accepted malformed frame")
		}
		if rs := replies(t, out.Bytes()); len(rs) != 1 || rs[0]["ok"] != false {
			t.Fatal(rs)
		}
	}
}
func TestUnicodeChunkExactOffsetsAndHash(t *testing.T) {
	content := strings.Repeat("😀漢字\n", 100000)
	offset := int64(0)
	var result strings.Builder
	hash := ""
	for {
		data, e := chunk(content, map[string]any{"offset": json.Number(fmtInt(offset)), "chunk_size": json.Number("160000")}, "c")
		if e != nil {
			t.Fatal(e)
		}
		result.WriteString(data["json_chunk"].(string))
		if data["offset"] != offset {
			t.Fatal(data)
		}
		if hash == "" {
			hash = data["document_hash"].(string)
		} else if hash != data["document_hash"] {
			t.Fatal(data)
		}
		offset = data["next_offset"].(int64)
		if data["done"] == true {
			break
		}
	}
	if result.String() != content || offset != 400000 {
		t.Fatal(offset)
	}
	if _, e := chunk(content, map[string]any{"offset": json.Number("400001")}, "x"); e == nil {
		t.Fatal("accepted excess offset")
	}
}
func fmtInt(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestProtocolAllCommandsCommitVisibilityAndUnicodeOriginals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	c := map[string]any{"schema_version": json.Number("1"), "url": "https://example.org/?q=1#state", "title": "Заголовок", "captured_at": "2026-10-27T12:00:00Z", "blocks": []any{map[string]any{"type": "text", "text": strings.Repeat("😀漢字\n", 50000)}}}
	requests := []map[string]any{{"id": "save", "type": "capture.save", "capture": c}, {"id": "alias", "type": "capture", "payload": c}, {"id": "list", "type": "pages.list", "query": "ЗАГОЛОВОК"}, {"id": "get", "type": "pages.get", "page_id": 1}, {"id": "capture", "type": "captures.get", "capture_id": 1, "chunk_size": 160000}, {"id": "export", "type": "pages.export", "page_id": 1, "chunk_size": 160000}, {"id": "domains", "type": "pages.domains"}, {"id": "stats", "type": "stats"}, {"id": "missing", "type": "pages.get", "page_id": 999}, {"id": "unknown", "type": "unknown"}, {"id": "delete", "type": "pages.delete", "page_id": 1}, {"id": "deleteagain", "type": "pages.delete", "page_id": 1}}
	var input, output bytes.Buffer
	for _, request := range requests {
		raw, e := archive.Canonical(request)
		if e != nil {
			t.Fatal(e)
		}
		input.Write(frame(raw))
	}
	if e := Serve(path, &input, &output); e != nil {
		t.Fatal(e)
	}
	rs := replies(t, output.Bytes())
	if len(rs) != len(requests) {
		t.Fatal(len(rs))
	}
	for i, r := range rs {
		if i == 8 || i == 9 {
			if r["ok"] != false {
				t.Fatal(i, r)
			}
		} else if r["ok"] != true {
			t.Fatal(i, r)
		}
	}
	if rs[1]["data"].(map[string]any)["created"] != false {
		t.Fatal(rs[1])
	}
	if rs[7]["data"].(map[string]any)["captures"] != float64(1) {
		t.Fatal(rs[7])
	}
	if rs[10]["data"].(map[string]any)["url"] != c["url"] {
		t.Fatal(rs[10])
	}
	if rs[11]["data"].(map[string]any)["url"] != nil {
		t.Fatal(rs[11])
	}
	stats, e := Diagnose(path)
	if e != nil || stats["captures"] != int64(0) || stats["blocks"] != int64(0) {
		t.Fatal(stats, e)
	}
}
func TestMaximumSummaryBatchesFitNativeFrames(t *testing.T) {
	s, e := archive.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	h := newHost(s)
	var pageID any
	url := ""
	for i := 0; i < 200; i++ {
		url = "https://example.org/" + strconv.Itoa(i) + "?q=" + strings.Repeat("x", 6000)
		c := map[string]any{"schema_version": 1, "url": url, "title": strings.Repeat("\x01", 1200), "captured_at": "2026-10-27T12:00:00Z", "reason": strings.Repeat("\x02", 1000), "blocks": []any{map[string]any{"type": "text", "text": "initial"}}}
		saved, e := s.Save(c)
		if e != nil {
			t.Fatal(e)
		}
		pageID = saved["page_id"]
	}
	listing := h.handle(map[string]any{"id": "list", "type": "pages.list", "limit": 200})
	if listing["ok"] != true {
		t.Fatal(listing)
	}
	if len(listing["data"].(map[string]any)["items"].([]map[string]any)) != 200 {
		t.Fatal("missing items")
	}
	for i := 0; i < 499; i++ {
		_, e = s.Save(map[string]any{"schema_version": 1, "url": url, "title": "title", "captured_at": "2026-10-27T12:00:00Z", "reason": strings.Repeat("\x03", 1000), "blocks": []any{map[string]any{"type": "text", "text": strconv.Itoa(i)}}})
		if e != nil {
			t.Fatal(e)
		}
	}
	detail := h.handle(map[string]any{"id": "detail", "type": "pages.get", "page_id": pageID, "limit": 500})
	if detail["ok"] != true {
		t.Fatal(detail)
	}
	if len(detail["data"].(map[string]any)["captures"].([]map[string]any)) != 500 {
		t.Fatal("missing captures")
	}
	for _, reply := range []map[string]any{listing, detail} {
		raw, e := archive.Canonical(reply)
		if e != nil || len(raw) >= MaxOutgoing {
			t.Fatal(len(raw), e)
		}
	}
}
func TestChunkDocumentHashTracksExportChanges(t *testing.T) {
	s, e := archive.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	h := newHost(s)
	c := map[string]any{"schema_version": 1, "url": "https://example.org/", "title": "A", "captured_at": "2026-10-27T12:00:00Z", "blocks": []any{map[string]any{"type": "text", "text": "unchanged"}}}
	saved, e := s.Save(c)
	if e != nil {
		t.Fatal(e)
	}
	a := h.handle(map[string]any{"id": "export", "type": "pages.export", "page_id": saved["page_id"], "chunk_size": 40})["data"].(map[string]any)
	c["title"] = "B"
	if _, e = s.Save(c); e != nil {
		t.Fatal(e)
	}
	b := h.handle(map[string]any{"id": "export", "type": "pages.export", "page_id": saved["page_id"], "chunk_size": 40})["data"].(map[string]any)
	if a["total_chars"] != b["total_chars"] || a["document_hash"] == b["document_hash"] {
		t.Fatal(a, b)
	}
}
func TestUnicodeReplacementIDRemainsCorrelatedOnInvalidCapture(t *testing.T) {
	var out bytes.Buffer
	raw := `{"id":"\ufffd","type":"capture","capture":{"title":"\ud800"}}`
	if e := Serve(filepath.Join(t.TempDir(), "db"), bytes.NewReader(frame(raw)), &out); e != nil {
		t.Fatal(e)
	}
	r := replies(t, out.Bytes())[0]
	if r["id"] != "�" || r["ok"] != false {
		t.Fatal(r)
	}
}
func TestRetainedPhysicalConnectionAcrossMixedRequests(t *testing.T) {
	s, e := archive.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.DB.Exec("CREATE TEMP TABLE session_marker(value TEXT); INSERT INTO session_marker VALUES('retained')"); e != nil {
		t.Fatal(e)
	}
	h := newHost(s)
	c := map[string]any{"schema_version": 1, "url": "https://example.org/", "title": "title", "captured_at": "2026-10-27T12:00:00Z", "blocks": []any{map[string]any{"type": "text", "text": "text"}}}
	requests := []map[string]any{{"id": "a", "type": "ping"}, {"id": "b", "type": "capture.save", "capture": c}, {"id": "c", "type": "pages.list"}, {"id": "d", "type": "pages.get", "page_id": 1}, {"id": "e", "type": "captures.get", "capture_id": 1}, {"id": "f", "type": "pages.export", "page_id": 1}, {"id": "g", "type": "pages.domains"}, {"id": "h", "type": "stats"}, {"id": "i", "type": "pages.delete", "page_id": 1}, {"id": "j", "type": "ping"}}
	for _, request := range requests {
		r := h.handle(request)
		if r["ok"] != true {
			t.Fatal(r)
		}
		var marker string
		if e = s.DB.QueryRow("SELECT value FROM session_marker").Scan(&marker); e != nil || marker != "retained" {
			t.Fatal("connection replaced", marker, e)
		}
	}
	if s.DB.Stats().OpenConnections != 1 {
		t.Fatal(s.DB.Stats())
	}
}
