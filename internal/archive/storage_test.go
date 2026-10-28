package archive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func capture() map[string]any {
	return map[string]any{"schema_version": json.Number("1"), "url": "https://gitlab.example/p?q=1#issue", "title": "Заголовок Straße", "lang": "ru", "captured_at": "2026-10-27T12:00:00.000Z", "reason": "initial", "source": map[string]any{"kind": "dom"}, "metadata": map[string]any{}, "blocks": []any{map[string]any{"type": "paragraph", "text": "исчезающий блок 😀"}}}
}
func store(t *testing.T) *Store {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "archive.sqlite3"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestSaveDedupAccumulationSearchDelete(t *testing.T) {
	s := store(t)
	c := capture()
	a, e := s.Save(c)
	if e != nil {
		t.Fatal(e)
	}
	c["captured_at"] = "2026-10-27T13:00:00Z"
	c["html"] = "changed"
	b, e := s.Save(c)
	if e != nil || b["created"] != false || a["capture_id"] != b["capture_id"] {
		t.Fatalf("dedup: %v %v", b, e)
	}
	c["blocks"] = []any{map[string]any{"type": "code", "text": "новый"}}
	if _, e = s.Save(c); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{"ИСЧЕЗАЮЩИЙ", "STRASSE"} {
		r, e := s.ListPages(q, "", 50, 0)
		if e != nil || r["total"] != int64(1) {
			t.Fatalf("search %s: %v %v", q, r, e)
		}
	}
	r, _ := s.ListPages("' OR 1=1 --", "", 50, 0)
	if r["total"] != int64(0) {
		t.Fatal(r)
	}
	x, e := s.Export(a["page_id"].(int64))
	if e != nil || len(x["blocks"].([]any)) != 2 {
		t.Fatalf("export %v %v", x, e)
	}
	d, e := s.Delete(a["page_id"].(int64))
	if e != nil || d["deleted"] != true || d["url"] != c["url"] {
		t.Fatal(d, e)
	}
	stats, _ := s.Stats()
	if stats["captures"] != int64(0) || stats["blocks"] != int64(0) {
		t.Fatal(stats)
	}
}
func TestRollbackAndSingleConnection(t *testing.T) {
	s := store(t)
	if _, e := s.DB.Exec("CREATE TEMP TABLE retained(value TEXT); INSERT INTO retained VALUES('same connection')"); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 4; i++ {
		if _, e := s.ListPages("", "", 50, 0); e != nil {
			t.Fatal(e)
		}
		if _, e := s.Stats(); e != nil {
			t.Fatal(e)
		}
	}
	var v string
	if e := s.DB.QueryRow("SELECT value FROM retained").Scan(&v); e != nil || v != "same connection" {
		t.Fatal(v, e)
	}
	if s.DB.Stats().OpenConnections != 1 {
		t.Fatal(s.DB.Stats())
	}
	if _, e := s.DB.Exec("CREATE TRIGGER deny_snapshot BEFORE INSERT ON captures BEGIN SELECT RAISE(ABORT,'simulated disk failure'); END"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Save(capture()); e == nil {
		t.Fatal("write unexpectedly succeeded")
	}
	stats, _ := s.Stats()
	if stats["pages"] != int64(0) {
		t.Fatal(stats)
	}
}
func TestValidationAndPrivateFile(t *testing.T) {
	s := store(t)
	for _, v := range []any{true, json.Number("2"), "1"} {
		c := capture()
		c["schema_version"] = v
		if _, e := s.Save(c); e == nil {
			t.Fatalf("accepted schema %v", v)
		}
	}
	for _, u := range []string{"javascript:alert(1)", "https://user:pass@example.org/", "https:///none"} {
		c := capture()
		c["url"] = u
		if _, e := s.Save(c); e == nil {
			t.Fatal(u)
		}
	}
	info, _ := os.Stat(s.Path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
}
func TestBoundedSummariesPreserveOriginal(t *testing.T) {
	s := store(t)
	c := capture()
	c["title"] = strings.Repeat("\x01", 2000)
	c["reason"] = strings.Repeat("Т", 1000)
	c["url"] = "https://example.org/?state=" + strings.Repeat("界", 65000)
	c["captured_at"] = "2026-10-27T12:00:00." + strings.Repeat("0", 10000) + "Z"
	saved, e := s.Save(c)
	if e != nil {
		t.Fatal(e)
	}
	detail, e := s.GetPage(saved["page_id"].(int64), 500, 0)
	if e != nil {
		t.Fatal(e)
	}
	p := detail["page"].(map[string]any)
	if p["url"] != "" || !strings.HasSuffix(p["title"].(string), "…") {
		t.Fatal(p)
	}
	raw, e := s.CaptureJSON(saved["capture_id"].(int64))
	if e != nil {
		t.Fatal(e)
	}
	var original map[string]any
	if e = json.Unmarshal([]byte(raw), &original); e != nil || original["url"] != c["url"] {
		t.Fatal(e)
	}
}
func TestTwoConnectionsAndParallelOpenCommitAllBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.sqlite3")
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func(i int) {
			s, e := Open(path)
			if e != nil {
				results <- e
				return
			}
			defer s.Close()
			c := capture()
			c["blocks"] = []any{map[string]any{"type": "text", "text": fmt.Sprintf("process-%d", i)}}
			_, e = s.Save(c)
			results <- e
		}(i)
	}
	for i := 0; i < 8; i++ {
		if e := <-results; e != nil {
			t.Fatal(e)
		}
	}
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	r, e := s.Stats()
	if e != nil || r["captures"] != int64(8) || r["blocks"] != int64(8) {
		t.Fatal(r, e)
	}
}

func TestLiteralDatabasePathWithURICharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "архив ?#%.sqlite3")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Save(capture()); e != nil {
		t.Fatal(e)
	}
	s.Close()
	data, e := os.ReadFile(path)
	if e != nil || !bytes.HasPrefix(data, []byte("SQLite format 3")) {
		t.Fatalf("requested DB was not written: %d bytes, %v", len(data), e)
	}
}
func TestNumericSchemaValidation(t *testing.T) {
	s := store(t)
	for _, v := range []any{json.Number("1e0"), json.Number("1.00"), []any{}, map[string]any{}} {
		c := capture()
		c["schema_version"] = v
		_, e := s.Save(c)
		switch v.(type) {
		case json.Number:
			if e != nil {
				t.Fatal(v, e)
			}
		default:
			if e == nil {
				t.Fatal("accepted malformed schema", v)
			}
		}
	}
}
func TestDeleteDoesNotTouchUnrelatedRows(t *testing.T) {
	s := store(t)
	saved, e := s.Save(capture())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec("INSERT INTO pages(id,url,title,hostname,first_seen,last_seen) VALUES(-1,'https://legacy.example/','legacy','legacy.example','old','old')"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Delete(saved["page_id"].(int64)); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = s.DB.QueryRow("SELECT count(*) FROM pages WHERE id=-1").Scan(&n); e != nil || n != 1 {
		t.Fatalf("unrelated row removed: %d %v", n, e)
	}
}
func TestTruncatedSummaryFieldsUseStableOrder(t *testing.T) {
	r := map[string]any{"title": strings.Repeat("x", 2000), "url": strings.Repeat("x", 4000), "hostname": strings.Repeat("x", 1000), "first_seen": strings.Repeat("x", 100), "last_seen": strings.Repeat("x", 100)}
	summary(r, pageBudgets)
	want := []string{"title", "url", "hostname", "first_seen", "last_seen"}
	if !reflect.DeepEqual(r["summary_truncated"], want) {
		t.Fatalf("got %v want %v", r["summary_truncated"], want)
	}
}
func TestReceiptIsCommittedAndSemanticURLChangesAreRevisions(t *testing.T) {
	s := store(t)
	a, e := s.Save(capture())
	if e != nil {
		t.Fatal(e)
	}
	other, e := Open(s.Path)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	stats, e := other.Stats()
	if e != nil || stats["captures"] != int64(1) {
		t.Fatal("receipt did not commit", stats, e)
	}
	c := capture()
	c["metadata"] = map[string]any{"state": "closed"}
	b, e := s.Save(c)
	if e != nil || b["capture_id"] == a["capture_id"] {
		t.Fatal(b, e)
	}
	c["url"] = "https://gitlab.example/p?q=2#issue"
	changed, e := s.Save(c)
	if e != nil || changed["page_id"] == a["page_id"] {
		t.Fatal(changed, e)
	}
}
