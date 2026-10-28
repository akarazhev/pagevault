package archive

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadOnlyArchiveReadsLiveCommitsAndRejectsWrites(t *testing.T) {
	w := store(t)
	saved, err := w.Save(capture())
	if err != nil {
		t.Fatal(err)
	}
	r, err := OpenReadOnly(w.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx := context.Background()
	pageID, captureID := saved["page_id"].(int64), saved["capture_id"].(int64)
	if got, err := r.ListPagesContext(ctx, "STRASSE", "GITLAB.EXAMPLE", 20, 0); err != nil || got["total"] != int64(1) {
		t.Fatal(got, err)
	}
	if got, err := r.GetPageContext(ctx, pageID, 20, 0); err != nil || got["total"] != int64(1) {
		t.Fatal(got, err)
	}
	want, _ := w.CaptureJSON(captureID)
	if got, err := r.CaptureJSONContext(ctx, captureID); err != nil || got != want {
		t.Fatal(got, err)
	}
	wantPage, _ := w.Export(pageID)
	if got, err := r.ExportContext(ctx, pageID); err != nil || !reflect.DeepEqual(got, wantPage) {
		t.Fatal(got, err)
	}
	var queryOnly int
	if err := r.DB.QueryRow("PRAGMA query_only").Scan(&queryOnly); err != nil || queryOnly != 1 {
		t.Fatal(queryOnly, err)
	}
	if _, err := r.Delete(pageID); err == nil {
		t.Fatal("read-only archive accepted deletion")
	}
	// mode=ro must still enforce read-only access if the advisory pragma is disabled.
	if _, err := r.DB.Exec("PRAGMA query_only=OFF"); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"DELETE FROM pages", "PRAGMA user_version=2", "CREATE TABLE forbidden(x)"} {
		if _, err := r.DB.Exec(q); err == nil {
			t.Fatal("read-only connection accepted", q)
		}
	}
	c := capture()
	c["blocks"] = []any{map[string]any{"type": "paragraph", "text": "new writer commit 😀"}}
	if _, err := w.Save(c); err != nil {
		t.Fatal(err)
	}
	if got, err := r.ListPagesContext(ctx, "new writer commit", "", 20, 0); err != nil || got["total"] != int64(1) {
		t.Fatal("reader cannot see live commit", got, err)
	}
	if got, err := r.GetPageContext(ctx, pageID, 20, 0); err != nil || got["total"] != int64(2) {
		t.Fatal(got, err)
	}
}

func TestReadOnlyArchiveNeverCreatesOrInitializes(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing", "archive.sqlite3")
	if s, err := OpenReadOnly(missing); err == nil {
		s.Close()
		t.Fatal("opened missing archive")
	}
	if _, err := os.Stat(filepath.Dir(missing)); !os.IsNotExist(err) {
		t.Fatal("created missing directory", err)
	}
	for _, version := range []int{0, 2, 99, -1, 1} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "archive.sqlite3")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(fmt.Sprintf("CREATE TABLE untouched(x); PRAGMA user_version=%d", version))
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if s, err := OpenReadOnly(path); err == nil {
				s.Close()
				t.Fatal("accepted unsupported or incomplete schema")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("changed existing archive", err)
			}
		})
	}
}

func TestReadOnlyArchiveTreatsURIPunctuationAsFilename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "архив ?mode=rw&_pragma=query_only(0)#%.sqlite3")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := w.Save(capture()); err != nil {
		t.Fatal(err)
	}
	r, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.Path != path {
		t.Fatal(r.Path)
	}
	if got, err := r.ListPages("", "", 20, 0); err != nil || got["total"] != int64(1) {
		t.Fatal(got, err)
	}
	if _, err := r.DB.Exec("PRAGMA query_only=OFF; DELETE FROM pages"); err == nil {
		t.Fatal("filename overrode read-only mode")
	}
}

func TestReadOnlySitesPaginationAndCancellation(t *testing.T) {
	w := store(t)
	for _, u := range []string{"https://b.example/1", "https://a.example/1", "https://a.example/2", "https://" + strings.Repeat("x", 2000) + ".example/"} {
		c := capture()
		c["url"] = u
		if _, err := w.Save(c); err != nil {
			t.Fatal(err)
		}
	}
	r, err := OpenReadOnly(w.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx := context.Background()
	got, err := r.ListSitesContext(ctx, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	items := got["items"].([]map[string]any)
	if got["total"] != int64(3) || len(items) != 1 || items[0]["hostname"] != "a.example" || items[0]["page_count"] != int64(2) {
		t.Fatal(got)
	}
	got, err = r.ListSitesContext(ctx, 20, 1)
	if err != nil {
		t.Fatal(err)
	}
	items = got["items"].([]map[string]any)
	if len(items) != 2 || items[0]["hostname"] != "b.example" || items[1]["summary_truncated"] == nil {
		t.Fatal(got)
	}
	for _, pair := range [][2]int64{{0, 0}, {201, 0}, {1, -1}} {
		if _, err := r.ListSitesContext(ctx, pair[0], pair[1]); err == nil {
			t.Fatal("accepted invalid pagination", pair)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.ListPagesContext(cancelled, "", "", 20, 0); err == nil {
		t.Fatal("ignored cancellation")
	}
	if _, err := r.ExportContext(cancelled, 1); err == nil {
		t.Fatal("ignored cancellation")
	}
}
