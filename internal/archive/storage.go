package archive

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/text/cases"
	"modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

type NotFoundError struct{ Message string }

func (e *NotFoundError) Error() string { return e.Message }
func invalid(f string, a ...any) error { return &ValidationError{fmt.Sprintf(f, a...)} }
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("casefold", 1, func(_ *sqlite.FunctionContext, a []driver.Value) (driver.Value, error) {
		if s, ok := a[0].(string); ok {
			return cases.Fold().String(s), nil
		}
		return "", nil
	})
}

type Store struct {
	DB   *sql.DB
	Path string
}

func DefaultPath() string {
	if p := os.Getenv("PAGEVAULT_DB"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		r := os.Getenv("LOCALAPPDATA")
		if r == "" {
			r = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(r, "PageVault", "archive.sqlite3")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "PageVault", "archive.sqlite3")
	default:
		r := os.Getenv("XDG_DATA_HOME")
		if r == "" {
			r = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(r, "pagevault", "archive.sqlite3")
	}
}
func resolvePath(path string) (string, error) {
	if path == "" {
		path = DefaultPath()
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		h, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		if path == "~" {
			path = h
		} else {
			path = filepath.Join(h, strings.TrimPrefix(path, "~/"))
		}
	}
	return filepath.Abs(path)
}

func sqliteURI(path string) *url.URL {
	// Encode URI punctuation so an ordinary filename cannot become DSN options.
	uriPath := filepath.ToSlash(path)
	if runtime.GOOS == "windows" && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	return &url.URL{Scheme: "file", Path: uriPath}
}

func Open(path string) (*Store, error) {
	p, e := resolvePath(path)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e == nil {
		e = f.Close()
	} else if errors.Is(e, os.ErrExist) {
		e = nil
	}
	if e != nil {
		return nil, e
	}
	dsn := sqliteURI(p).String()
	db, e := sql.Open("sqlite", dsn)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	s := &Store{DB: db, Path: p}
	fail := func(e error) (*Store, error) { db.Close(); return nil, e }
	for _, q := range []string{"PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=30000"} {
		if _, e = db.Exec(q); e != nil {
			return fail(e)
		}
	}
	if e = prepareSchema(db); e != nil {
		return fail(e)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, e = db.Exec("PRAGMA journal_mode=WAL")
		if e == nil {
			break
		}
		text := strings.ToLower(e.Error())
		if (!strings.Contains(text, "busy") && !strings.Contains(text, "locked")) || time.Now().After(deadline) {
			return fail(e)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, e = db.Exec("PRAGMA synchronous=FULL"); e != nil {
		return fail(e)
	}
	return s, nil
}
func (s *Store) Close() error { return s.DB.Close() }
func Hash(raw string) string  { h := sha256.Sum256([]byte(raw)); return hex.EncodeToString(h[:]) }
func Integer(v any, name string, min, max int64) (int64, error) {
	var n int64
	var e error
	switch x := v.(type) {
	case json.Number:
		if strings.ContainsAny(string(x), ".eE") {
			e = fmt.Errorf("fraction")
		} else {
			n, e = strconv.ParseInt(string(x), 10, 64)
		}
	case int:
		n = int64(x)
	case int64:
		n = x
	default:
		e = fmt.Errorf("type")
	}
	if e != nil || n < min || (max >= 0 && n > max) {
		upper := "unbounded"
		if max >= 0 {
			upper = strconv.FormatInt(max, 10)
		}
		return 0, invalid("%s must be an integer between %d and %s", name, min, upper)
	}
	return n, nil
}
func validate(c map[string]any) (string, string, error) {
	if c == nil {
		return "", "", invalid("capture.schema_version must be 1")
	}
	schema := c["schema_version"]
	valid := schema == 1 || schema == int64(1) || schema == float64(1)
	if number, ok := schema.(json.Number); ok {
		f, err := strconv.ParseFloat(string(number), 64)
		valid = err == nil && f == 1
	}
	if !valid {
		return "", "", invalid("capture.schema_version must be 1")
	}
	for _, k := range []string{"url", "title", "captured_at"} {
		if _, ok := c[k].(string); !ok {
			return "", "", invalid("capture.%s must be text", k)
		}
	}
	u := c["url"].(string)
	if utf8.RuneCountInString(u) > 65536 {
		return "", "", invalid("URL too long")
	}
	parsed, e := url.Parse(u)
	if e != nil || parsed == nil || !(strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) || parsed.Hostname() == "" || parsed.User != nil {
		return "", "", invalid("capture.url must be an HTTP(S) URL without credentials")
	}
	stamp := c["captured_at"].(string)
	if !validTimestamp(stamp) {
		return "", "", invalid("capture.captured_at must be an RFC3339 timestamp with timezone")
	}
	for _, k := range []string{"lang", "reason", "session_id", "html"} {
		if v, ok := c[k]; ok {
			if _, ok = v.(string); !ok {
				return "", "", invalid("capture.%s must be text", k)
			}
		}
	}
	for _, k := range []string{"source", "metadata", "stats"} {
		if v, ok := c[k]; ok {
			if _, ok = v.(map[string]any); !ok {
				return "", "", invalid("capture.%s must be an object", k)
			}
		}
	}
	blocks, ok := c["blocks"].([]any)
	if !ok || len(blocks) > 50000 {
		return "", "", invalid("capture.blocks must be an array with at most 50000 blocks")
	}
	for _, v := range blocks {
		b, ok := v.(map[string]any)
		if !ok {
			return "", "", invalid("each block needs a text type")
		}
		ty, ok := b["type"].(string)
		if !ok || ty == "" {
			return "", "", invalid("each block needs a text type")
		}
		if tx, ok := b["text"]; ok {
			if _, ok = tx.(string); !ok {
				return "", "", invalid("block.text must be text")
			}
		}
	}
	if ws, ok := c["warnings"]; ok {
		a, ok := ws.([]any)
		if !ok {
			return "", "", invalid("capture.warnings must contain code/message objects")
		}
		for _, v := range a {
			w, ok := v.(map[string]any)
			if !ok {
				return "", "", invalid("capture.warnings must contain code/message objects")
			}
			_, a := w["code"].(string)
			_, b := w["message"].(string)
			if !a || !b {
				return "", "", invalid("capture.warnings must contain code/message objects")
			}
		}
	}
	raw, e := Canonical(c)
	if e != nil {
		return "", "", invalid("capture must contain finite JSON values: %v", e)
	}
	if len(raw) > 32*1024*1024 {
		return "", "", invalid("capture exceeds 32 MiB")
	}
	return strings.ToLower(parsed.Hostname()), raw, nil
}
func blockText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		p := make([]string, len(x))
		for i, v := range x {
			p[i] = blockText(v)
		}
		return strings.Join(p, " ")
	case map[string]any:
		p := make([]string, 0, len(x))
		for _, v := range x {
			p = append(p, blockText(v))
		}
		return strings.Join(p, " ")
	}
	return ""
}
func (s *Store) Save(c map[string]any) (map[string]any, error) {
	host, raw, e := validate(c)
	if e != nil {
		return nil, e
	}
	semantic := map[string]any{}
	for _, k := range []string{"schema_version", "url", "title", "lang", "source", "metadata", "blocks"} {
		semantic[k] = c[k]
	}
	identity, e := Canonical(semantic)
	if e != nil {
		return nil, e
	}
	digest := Hash(identity)
	blocks := c["blocks"].([]any)
	stamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	tx, e := s.DB.Begin()
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	if _, e = tx.Exec("INSERT INTO pages(url,title,hostname,first_seen,last_seen) VALUES(?,?,?,?,?) ON CONFLICT(url) DO UPDATE SET title=excluded.title,last_seen=excluded.last_seen", c["url"], c["title"], host, stamp, stamp); e != nil {
		return nil, e
	}
	var pageID, captureID int64
	if e = tx.QueryRow("SELECT id FROM pages WHERE url=?", c["url"]).Scan(&pageID); e != nil {
		return nil, e
	}
	reason, _ := c["reason"].(string)
	r, e := tx.Exec("INSERT OR IGNORE INTO captures(page_id,captured_at,content_hash,block_count,reason,payload_json) VALUES(?,?,?,?,?,?)", pageID, c["captured_at"], digest, len(blocks), reason, raw)
	if e != nil {
		return nil, e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return nil, e
	}
	if e = tx.QueryRow("SELECT id FROM captures WHERE page_id=? AND content_hash=?", pageID, digest).Scan(&captureID); e != nil {
		return nil, e
	}
	for _, block := range blocks {
		content, e := Canonical(block)
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec("INSERT OR IGNORE INTO blocks(page_id,block_hash,payload_json,search_text) VALUES(?,?,?,?)", pageID, Hash(content), content, blockText(block)); e != nil {
			return nil, e
		}
	}
	if _, e = tx.Exec("UPDATE pages SET latest_capture_id=? WHERE id=?", captureID, pageID); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return map[string]any{"page_id": pageID, "capture_id": captureID, "created": n == 1, "block_count": int64(len(blocks))}, nil
}

const pageColumns = `p.id,p.url,p.title,p.hostname,p.first_seen,p.last_seen,p.latest_capture_id,(SELECT count(*) FROM captures c WHERE c.page_id=p.id) AS capture_count,(SELECT count(*) FROM blocks b WHERE b.page_id=p.id) AS block_count`

func readRows(rows *sql.Rows) ([]map[string]any, error) {
	defer rows.Close()
	cols, e := rows.Columns()
	if e != nil {
		return nil, e
	}
	items := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptr := make([]any, len(cols))
		for i := range vals {
			ptr[i] = &vals[i]
		}
		if e = rows.Scan(ptr...); e != nil {
			return nil, e
		}
		m := map[string]any{}
		for i, c := range cols {
			m[c] = vals[i]
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

var pageBudgets = map[string]int{"title": 1024, "url": 2048, "hostname": 256, "first_seen": 64, "last_seen": 64}
var captureBudgets = map[string]int{"captured_at": 64, "reason": 256}

func bounded(s string, budget int) (string, bool) {
	raw, _ := Canonical(s)
	if len(raw) <= budget {
		return s, false
	}
	r := []rune(s)
	lo, hi := 0, min(len(r), budget)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		raw, _ = Canonical(string(r[:mid]) + "…")
		if len(raw) <= budget {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(r[:lo]) + "…", true
}
func summary(m map[string]any, budgets map[string]int) map[string]any {
	fields := []string{}
	for _, k := range []string{"title", "url", "hostname", "first_seen", "last_seen", "captured_at", "reason"} {
		budget, exists := budgets[k]
		if !exists {
			continue
		}
		if s, ok := m[k].(string); ok {
			v, changed := bounded(s, budget)
			if changed {
				fields = append(fields, k)
				if k == "url" {
					m["url_display"] = v
					m[k] = ""
				} else {
					m[k] = v
				}
			}
		}
	}
	if len(fields) > 0 {
		m["summary_truncated"] = fields
	}
	return m
}
func (s *Store) ListPages(query, domain string, limit, offset int64) (map[string]any, error) {
	return s.ListPagesContext(context.Background(), query, domain, limit, offset)
}
func (s *Store) ListPagesContext(ctx context.Context, query, domain string, limit, offset int64) (map[string]any, error) {
	if _, e := Integer(limit, "limit", 1, 200); e != nil {
		return nil, e
	}
	if _, e := Integer(offset, "offset", 0, -1); e != nil {
		return nil, e
	}
	conditions := []string{}
	args := []any{}
	if query != "" {
		conditions = append(conditions, "(instr(casefold(p.title),casefold(?))>0 OR instr(casefold(p.url),casefold(?))>0 OR EXISTS(SELECT 1 FROM blocks b WHERE b.page_id=p.id AND instr(casefold(b.search_text),casefold(?))>0))")
		args = append(args, query, query, query)
	}
	if domain != "" {
		conditions = append(conditions, "p.hostname=?")
		args = append(args, strings.ToLower(domain))
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	var total int64
	if e := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM pages p"+where, args...).Scan(&total); e != nil {
		return nil, e
	}
	rows, e := s.DB.QueryContext(ctx, "SELECT "+pageColumns+" FROM pages p"+where+" ORDER BY p.last_seen DESC,p.id DESC LIMIT ? OFFSET ?", append(args, limit, offset)...)
	if e != nil {
		return nil, e
	}
	items, e := readRows(rows)
	if e != nil {
		return nil, e
	}
	for _, p := range items {
		summary(p, pageBudgets)
	}
	return map[string]any{"items": items, "total": total, "limit": limit, "offset": offset}, nil
}
func (s *Store) Page(id int64, bounded bool) (map[string]any, error) {
	return readPage(context.Background(), s.DB, id, bounded)
}

type pageReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readPage(ctx context.Context, db pageReader, id int64, bounded bool) (map[string]any, error) {
	if _, e := Integer(id, "page_id", 1, -1); e != nil {
		return nil, e
	}
	rows, e := db.QueryContext(ctx, "SELECT "+pageColumns+" FROM pages p WHERE p.id=?", id)
	if e != nil {
		return nil, e
	}
	items, e := readRows(rows)
	if e != nil {
		return nil, e
	}
	if len(items) == 0 {
		return nil, &NotFoundError{"Page not found"}
	}
	if bounded {
		return summary(items[0], pageBudgets), nil
	}
	return items[0], nil
}
func (s *Store) GetPage(id, limit, offset int64) (map[string]any, error) {
	return s.GetPageContext(context.Background(), id, limit, offset)
}
func (s *Store) GetPageContext(ctx context.Context, id, limit, offset int64) (map[string]any, error) {
	if _, e := Integer(limit, "limit", 1, 500); e != nil {
		return nil, e
	}
	if _, e := Integer(offset, "offset", 0, -1); e != nil {
		return nil, e
	}
	page, e := readPage(ctx, s.DB, id, true)
	if e != nil {
		return nil, e
	}
	rows, e := s.DB.QueryContext(ctx, "SELECT id,captured_at,content_hash,block_count,reason FROM captures WHERE page_id=? ORDER BY id DESC LIMIT ? OFFSET ?", id, limit, offset)
	if e != nil {
		return nil, e
	}
	items, e := readRows(rows)
	if e != nil {
		return nil, e
	}
	for _, c := range items {
		summary(c, captureBudgets)
	}
	return map[string]any{"page": page, "captures": items, "total": page["capture_count"]}, nil
}
func (s *Store) CaptureJSON(id int64) (string, error) {
	return s.CaptureJSONContext(context.Background(), id)
}
func (s *Store) CaptureJSONContext(ctx context.Context, id int64) (string, error) {
	if _, e := Integer(id, "capture_id", 1, -1); e != nil {
		return "", e
	}
	var raw string
	e := s.DB.QueryRowContext(ctx, "SELECT payload_json FROM captures WHERE id=?", id).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return "", &NotFoundError{"Capture not found"}
	}
	return raw, e
}
func (s *Store) Export(id int64) (map[string]any, error) {
	return s.ExportContext(context.Background(), id)
}
func (s *Store) ExportContext(ctx context.Context, id int64) (map[string]any, error) {
	// Read metadata and accumulated blocks from one committed snapshot.
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	page, e := readPage(ctx, tx, id, false)
	if e != nil {
		return nil, e
	}
	rows, e := tx.QueryContext(ctx, "SELECT payload_json FROM blocks WHERE page_id=? ORDER BY id", id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	blocks := []any{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		v, e := Decode(raw)
		if e != nil {
			return nil, e
		}
		blocks = append(blocks, v)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if e = rows.Close(); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return map[string]any{"schema_version": 1, "archive_kind": "observed_blocks", "page": page, "blocks": blocks}, nil
}
func (s *Store) Domains() (map[string]any, error) { return s.DomainsPage(200, 0) }

func (s *Store) DomainsPage(limit, offset int64) (map[string]any, error) {
	if _, err := Integer(limit, "limit", 1, 200); err != nil {
		return nil, err
	}
	if _, err := Integer(offset, "offset", 0, -1); err != nil {
		return nil, err
	}
	var total int64
	if err := s.DB.QueryRow("SELECT count(DISTINCT hostname) FROM pages").Scan(&total); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query("SELECT hostname,count(*) FROM pages GROUP BY hostname ORDER BY hostname LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	size := 0
	for rows.Next() {
		var hostname string
		var count int64
		if err = rows.Scan(&hostname, &count); err != nil {
			return nil, err
		}
		item := map[string]any{"hostname": hostname, "page_count": count}
		raw, err := Canonical(item)
		if err != nil {
			return nil, err
		}
		if size+len(raw)+1 > 760*1024 {
			if len(items) == 0 {
				return nil, fmt.Errorf("hostname exceeds response budget")
			}
			break
		}
		size += len(raw) + 1
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var next any
	if offset+int64(len(items)) < total && len(items) > 0 {
		next = offset + int64(len(items))
	}
	return map[string]any{"items": items, "total": total, "limit": limit, "offset": offset, "next_offset": next}, nil
}
func (s *Store) Stats() (map[string]any, error) {
	m := map[string]any{"db_path": s.Path}
	for _, table := range []string{"pages", "captures", "blocks"} {
		var n int64
		if e := s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); e != nil {
			return nil, e
		}
		m[table] = n
	}
	var size int64
	for _, p := range []string{s.Path, s.Path + "-wal"} {
		if i, e := os.Stat(p); e == nil {
			size += i.Size()
		}
	}
	m["db_bytes"] = size
	return m, nil
}
func (s *Store) Delete(id int64) (map[string]any, error) {
	if _, e := Integer(id, "page_id", 1, -1); e != nil {
		return nil, e
	}
	ctx := context.Background()
	conn, e := s.DB.Conn(ctx)
	if e != nil {
		return nil, e
	}
	defer conn.Close()
	if _, e = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); e != nil {
		return nil, e
	}
	defer conn.ExecContext(ctx, "ROLLBACK")
	var u string
	e = conn.QueryRowContext(ctx, "SELECT url FROM pages WHERE id=?", id).Scan(&u)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	var original any
	if e == nil {
		original = u
	}
	r, e := conn.ExecContext(ctx, "DELETE FROM pages WHERE id=?", id)
	if e != nil {
		return nil, e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return nil, e
	}
	if _, e = conn.ExecContext(ctx, "COMMIT"); e != nil {
		return nil, e
	}
	return map[string]any{"deleted": n > 0, "url": original}, nil
}
