package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"pagevault.local/pagevault/internal/archive"
	"pagevault.local/pagevault/internal/buildinfo"
)

func fixture(t *testing.T) (*archive.Store, *mcp.ClientSession, context.Context) {
	t.Helper()
	w, err := archive.Open(filepath.Join(t.TempDir(), "archive.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	r, err := archive.OpenReadOnly(w.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	ct, st := mcp.NewInMemoryTransports()
	ss, err := newServer(r).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "pagevault-test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return w, cs, ctx
}

func capture(text string) map[string]any {
	return map[string]any{
		"schema_version": 1, "url": "https://example.org/docs", "title": "Документ Straße", "captured_at": "2026-10-28T12:00:00Z",
		"metadata": map[string]any{"exact": json.Number("9007199254740993"), "decimal": json.Number("1.2300")},
		"html":     "<script>untrusted HTML</script>", "warnings": []any{map[string]any{"code": "partial", "message": "Only the observed part was saved"}},
		"blocks": []any{map[string]any{"type": "paragraph", "text": text}, map[string]any{"type": "table", "rows": []any{[]any{"Имя", "Значение"}}}, map[string]any{"type": "code", "text": "print(42)", "language": "python"}},
	}
}

func save(t *testing.T, w *archive.Store, c map[string]any) (int64, int64) {
	t.Helper()
	r, err := w.Save(c)
	if err != nil {
		t.Fatal(err)
	}
	return r["page_id"].(int64), r["capture_id"].(int64)
}

func call(t *testing.T, cs *mcp.ClientSession, ctx context.Context, name string, args map[string]any) map[string]any {
	t.Helper()
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError {
		t.Fatalf("%s: %v", name, r.Content)
	}
	raw, err := json.Marshal(r.StructuredContent)
	if err != nil || len(raw) > 32*1024 {
		t.Fatalf("unbounded tool data: %d bytes, %v", len(raw), err)
	}
	v, err := archive.Decode(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Content) != 1 {
		t.Fatalf("missing text fallback: %v", r.Content)
	}
	text, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("missing JSON text fallback")
	}
	fallback, err := archive.Decode(text.Text)
	if err != nil || !reflect.DeepEqual(v, fallback) {
		t.Fatal("text/structured content differ", err)
	}
	return v.(map[string]any)
}

func integer(t *testing.T, value any) int64 {
	t.Helper()
	n, err := archive.Integer(value, "test value", 0, -1)
	if err != nil {
		t.Fatal(value, err)
	}
	return n
}

func TestSDKDiscoveryAndReadOnlyTools(t *testing.T) {
	w, cs, ctx := fixture(t)
	pageID, captureID := save(t, w, capture("Исчезающий текст 😀"))
	init := cs.InitializeResult()
	if init.ServerInfo.Version != buildinfo.Version || !strings.Contains(strings.ToLower(init.Instructions), "untrusted") {
		t.Fatal(init)
	}
	if init.Capabilities.Resources != nil || init.Capabilities.Prompts != nil {
		t.Fatal("advertised unsupported capabilities", init.Capabilities)
	}
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
		a := tool.Annotations
		if a == nil || !a.ReadOnlyHint || !a.IdempotentHint || a.DestructiveHint == nil || *a.DestructiveHint || a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Fatal("wrong annotations", tool)
		}
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatal("missing tool schema", tool.Name)
		}
	}
	if !reflect.DeepEqual(names, map[string]bool{"search_pages": true, "get_page": true, "list_versions": true, "get_version": true, "list_sites": true}) {
		t.Fatal(names)
	}
	pages := call(t, cs, ctx, "search_pages", map[string]any{"query": "STRASSE", "domain": "EXAMPLE.ORG"})
	if integer(t, pages["total"]) != 1 || integer(t, pages["limit"]) != 20 {
		t.Fatal(pages)
	}
	if integer(t, pages["items"].([]any)[0].(map[string]any)["id"]) != pageID {
		t.Fatal(pages)
	}
	versions := call(t, cs, ctx, "list_versions", map[string]any{"page_id": pageID})
	if integer(t, versions["items"].([]any)[0].(map[string]any)["id"]) != captureID {
		t.Fatal(versions)
	}
	sites := call(t, cs, ctx, "list_sites", map[string]any{})
	if sites["items"].([]any)[0].(map[string]any)["hostname"] != "example.org" {
		t.Fatal(sites)
	}
	if r := call(t, cs, ctx, "search_pages", map[string]any{"query": "' OR 1=1 --"}); integer(t, r["total"]) != 0 {
		t.Fatal(r)
	}
}

func assemble(t *testing.T, cs *mcp.ClientSession, ctx context.Context, name string, args map[string]any) string {
	t.Helper()
	var text strings.Builder
	var hash string
	for i := 0; i < 1000; i++ {
		r := call(t, cs, ctx, name, args)
		if integer(t, r["offset"]) != int64(utf8.RuneCountInString(text.String())) {
			t.Fatal("wrong rune offset", r)
		}
		if hash != "" && r["document_hash"] != hash {
			t.Fatal("document changed", r)
		}
		hash = r["document_hash"].(string)
		chunk := r["text"].(string)
		if r["done"] != (r["next_offset"] == nil) {
			t.Fatal("missing or inconsistent completion marker", r["done"])
		}
		if !utf8.ValidString(chunk) {
			t.Fatal("split UTF-8 character")
		}
		text.WriteString(chunk)
		if r["next_offset"] == nil {
			if integer(t, r["total_chars"]) != int64(utf8.RuneCountInString(text.String())) || archive.Hash(text.String()) != hash {
				t.Fatal("incomplete document")
			}
			return text.String()
		}
		if chunk == "" {
			t.Fatal("pagination made no progress")
		}
		args["offset"], args["document_hash"] = r["next_offset"], hash
	}
	t.Fatal("too many chunks")
	return ""
}

func TestDocumentChunksPreserveStructureNumbersAndVersionHistory(t *testing.T) {
	w, cs, ctx := fixture(t)
	original := capture(strings.Repeat("😀界\n\"<>&\x01", 3000))
	pageID, captureID := save(t, w, original)
	accumulated := assemble(t, cs, ctx, "get_page", map[string]any{"page_id": pageID, "max_chars": 16000})
	v, err := archive.Decode(accumulated)
	if err != nil || v.(map[string]any)["archive_kind"] != "observed_blocks" || len(v.(map[string]any)["blocks"].([]any)) != 3 {
		t.Fatal(v, err)
	}
	version := assemble(t, cs, ctx, "get_version", map[string]any{"capture_id": captureID, "max_chars": 16000})
	v, err = archive.Decode(version)
	if err != nil {
		t.Fatal(err)
	}
	doc := v.(map[string]any)
	if _, exists := doc["html"]; exists {
		t.Fatal("exposed stored HTML")
	}
	delete(original, "html")
	want, err := archive.Canonical(original)
	if err != nil || version != want {
		t.Fatal("lost structure, metadata, warnings or exact numeric tokens", err)
	}
	changed := capture("new snapshot")
	save(t, w, changed)
	if got := assemble(t, cs, ctx, "get_version", map[string]any{"capture_id": captureID, "max_chars": 3000}); got != version {
		t.Fatal("immutable snapshot changed")
	}
	r := call(t, cs, ctx, "get_page", map[string]any{"page_id": pageID})
	if integer(t, r["total_chars"]) <= 8000 || utf8.RuneCountInString(r["text"].(string)) > 8000 {
		t.Fatal("wrong default chunk size", r)
	}
}

func TestAccumulatedDocumentHashPreventsMixingBrowserCommits(t *testing.T) {
	w, cs, ctx := fixture(t)
	pageID, _ := save(t, w, capture(strings.Repeat("old block 😀", 1000)))
	first := call(t, cs, ctx, "get_page", map[string]any{"page_id": pageID, "max_chars": 100})
	save(t, w, capture("new block"))
	for _, args := range []map[string]any{
		{"page_id": pageID, "offset": first["next_offset"]},
		{"page_id": pageID, "offset": first["next_offset"], "document_hash": first["document_hash"]},
	} {
		r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_page", Arguments: args})
		if err != nil || !r.IsError {
			t.Fatal("accepted unsafe continuation", r, err)
		}
	}
	current := assemble(t, cs, ctx, "get_page", map[string]any{"page_id": pageID})
	if !strings.Contains(current, "old block") || !strings.Contains(current, "new block") {
		t.Fatal("lost accumulated blocks")
	}
}

func TestVersionContinuationRequiresHashWhenDeletedIDsAreReused(t *testing.T) {
	w, cs, ctx := fixture(t)
	pageID, captureID := save(t, w, capture(strings.Repeat("original", 200)))
	first := call(t, cs, ctx, "get_version", map[string]any{"capture_id": captureID, "max_chars": 100})
	if _, err := w.Delete(pageID); err != nil {
		t.Fatal(err)
	}
	_, replacementID := save(t, w, capture(strings.Repeat("replacement", 200)))
	if replacementID != captureID {
		t.Fatal("fixture did not reuse SQLite INTEGER PRIMARY KEY")
	}
	for _, args := range []map[string]any{
		{"capture_id": captureID, "offset": first["next_offset"]},
		{"capture_id": captureID, "offset": first["next_offset"], "document_hash": first["document_hash"]},
	} {
		r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_version", Arguments: args})
		if err != nil || !r.IsError {
			t.Fatal("unsafe continuation of a replaced capture", r, err)
		}
	}
}

func TestListsBoundBytesAndKeepPagination(t *testing.T) {
	w, cs, ctx := fixture(t)
	for i := 0; i < 40; i++ {
		c := capture(fmt.Sprint(i))
		c["title"] = strings.Repeat("\x01", 2000)
		c["url"] = fmt.Sprintf("https://site-%02d.example/", i) + strings.Repeat("界", 1000)
		save(t, w, c)
	}
	seen := map[int64]bool{}
	var offset any = 0
	for {
		r := call(t, cs, ctx, "search_pages", map[string]any{"limit": 100, "offset": offset})
		if integer(t, r["total"]) != 40 {
			t.Fatal(r)
		}
		for _, v := range r["items"].([]any) {
			p := v.(map[string]any)
			id := integer(t, p["id"])
			if seen[id] || p["url"] != "" || p["url_display"] == nil || p["summary_truncated"] == nil {
				t.Fatal(p)
			}
			seen[id] = true
		}
		if r["next_offset"] == nil {
			break
		}
		if integer(t, r["next_offset"]) <= integer(t, mustNumber(offset)) {
			t.Fatal("pagination stalled")
		}
		offset = r["next_offset"]
	}
	if len(seen) != 40 {
		t.Fatal("pagination lost pages", len(seen))
	}
	for _, name := range []string{"search_pages", "list_sites"} {
		r := call(t, cs, ctx, name, map[string]any{"limit": 1, "offset": 39})
		if len(r["items"].([]any)) != 1 || r["next_offset"] != nil {
			t.Fatal(r)
		}
		r = call(t, cs, ctx, name, map[string]any{"offset": 1000})
		if len(r["items"].([]any)) != 0 || r["next_offset"] != nil {
			t.Fatal(r)
		}
	}
}

func mustNumber(value any) any {
	if n, ok := value.(int); ok {
		return json.Number(fmt.Sprint(n))
	}
	return value
}

func TestInvalidRequestsAndNoMutationTools(t *testing.T) {
	w, cs, ctx := fixture(t)
	pageID, captureID := save(t, w, capture("preserved"))
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"search_pages", map[string]any{"limit": 0}}, {"list_sites", map[string]any{"limit": 101}}, {"search_pages", map[string]any{"offset": -1}},
		{"search_pages", map[string]any{"limit": 1.5}}, {"search_pages", map[string]any{"db": "other.sqlite3"}},
		{"get_page", map[string]any{}}, {"get_page", map[string]any{"page_id": 0}}, {"get_page", map[string]any{"page_id": 999}},
		{"get_page", map[string]any{"page_id": pageID, "max_chars": 16001}}, {"get_page", map[string]any{"page_id": pageID, "max_chars": 0}},
		{"get_page", map[string]any{"page_id": pageID, "offset": -1}}, {"get_page", map[string]any{"page_id": pageID, "offset": 999999, "document_hash": "wrong"}},
		{"get_version", map[string]any{"capture_id": 999}}, {"get_version", map[string]any{"capture_id": captureID, "offset": 999999}},
		{"list_versions", map[string]any{"page_id": pageID, "limit": 101}}, {"list_versions", map[string]any{"page_id": 999}},
		{"delete_page", map[string]any{"page_id": pageID}}, {"execute_sql", map[string]any{"sql": "DELETE FROM pages"}},
	} {
		t.Run(tc.name+fmt.Sprint(tc.args), func(t *testing.T) {
			r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err == nil && !r.IsError {
				t.Fatal("accepted invalid request", r)
			}
		})
	}
	stats, err := w.Stats()
	if err != nil || stats["pages"] != int64(1) || stats["captures"] != int64(1) {
		t.Fatal(stats, err)
	}
}

func TestVersionSummaryPaginationIsBounded(t *testing.T) {
	w, cs, ctx := fixture(t)
	var pageID int64
	for i := 0; i < 100; i++ {
		c := capture(fmt.Sprint(i))
		c["reason"] = strings.Repeat("\x01", 1000)
		pageID, _ = save(t, w, c)
	}
	var offset any = 0
	var previous int64 = 101
	count := 0
	for {
		r := call(t, cs, ctx, "list_versions", map[string]any{"page_id": pageID, "limit": 100, "offset": offset})
		if integer(t, r["total"]) != 100 {
			t.Fatal(r)
		}
		for _, item := range r["items"].([]any) {
			v := item.(map[string]any)
			id := integer(t, v["id"])
			if id >= previous || v["summary_truncated"] == nil {
				t.Fatal("wrong capture order or bounds", v)
			}
			previous = id
			count++
		}
		if r["next_offset"] == nil {
			break
		}
		offset = r["next_offset"]
	}
	if count != 100 {
		t.Fatal("lost versions", count)
	}
}

func TestServeBoundsInboundFramesAndSupportsCancellation(t *testing.T) {
	w, _, _ := fixture(t)
	var out bytes.Buffer
	if err := Serve(context.Background(), w.Path, strings.NewReader(strings.Repeat(" ", 65*1024)+"\n"), &out); err == nil {
		t.Fatal("accepted oversized input frame")
	}
	if out.Len() != 0 {
		t.Fatal("wrote non-protocol output", out.String())
	}
	input, peer := io.Pipe()
	defer peer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, w.Path, input, io.Discard) }()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		input.Close()
		t.Fatal("server did not stop after cancellation")
	}
}

func TestWirePreservesLargeIDsAndBoundsAfterInitialize(t *testing.T) {
	w, _, _ := fixture(t)
	page, capid := save(t, w, capture("synthetic"))
	const high int64 = 9007199254740993
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"PRAGMA foreign_keys=OFF", nil},
		{"UPDATE pages SET id=? WHERE id=?", []any{high, page}},
		{"UPDATE captures SET page_id=?,id=? WHERE id=?", []any{high, high, capid}},
		{"UPDATE blocks SET page_id=?", []any{high}},
		{"PRAGMA foreign_keys=ON", nil},
	} {
		if _, err := w.DB.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	input, writer := io.Pipe()
	output, reader := io.Pipe()
	defer input.Close()
	defer reader.Close()
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), w.Path, input, reader) }()
	scan := bufio.NewScanner(output)
	scan.Buffer(make([]byte, 4096), 1024*1024)
	request := func(s string) string {
		t.Helper()
		if _, err := io.WriteString(writer, s+"\n"); err != nil {
			t.Fatal(err)
		}
		if !scan.Scan() {
			t.Fatal("missing reply", scan.Err())
		}
		return scan.Text()
	}
	request(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"audit","version":"1"},"capabilities":{}}}`)
	for i, call := range []string{`{"name":"search_pages","arguments":{}}`, `{"name":"list_versions","arguments":{"page_id":9007199254740993}}`, `{"name":"get_page","arguments":{"page_id":9007199254740993}}`, `{"name":"get_version","arguments":{"capture_id":9007199254740993}}`} {
		reply := request(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":%s}`, i+2, call))
		if strings.Count(reply, "9007199254740993") < 2 || strings.Contains(reply, "9007199254740992") || strings.Contains(reply, `"isError":true`) {
			t.Fatalf("lost ID: %s", reply)
		}
	}
	writer.Close()
	<-done
}

func TestWireRejectsOversizedFrameAfterShortFrame(t *testing.T) {
	w, _, _ := fixture(t)
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"audit","version":"1"},"capabilities":{}}}` + "\n"
	oversized := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"ping","params":{"_meta":{"padding":%q}}}`+"\n", strings.Repeat("x", 65500))
	var out bytes.Buffer
	err := Serve(context.Background(), w.Path, strings.NewReader(init+oversized), &out)
	if err == nil {
		t.Fatal("accepted oversized second frame")
	}
}
