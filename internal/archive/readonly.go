package archive

import (
	"context"
	"database/sql"
	"fmt"
)

// OpenReadOnly opens an existing schema-1 archive without initializing it or
// changing its journal mode. mode=ro enforces this even without query_only.
// Do not use immutable=1: the browser may commit new captures while MCP reads.
func OpenReadOnly(path string) (*Store, error) {
	p, err := resolvePath(path)
	if err != nil {
		return nil, err
	}
	u := sqliteURI(p)
	q := u.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", "busy_timeout(30000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(err error) (*Store, error) {
		db.Close()
		return nil, fmt.Errorf("open existing archive read-only: %w", err)
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version != 1 {
		return fail(fmt.Errorf("unsupported archive schema version %d (supported: 1); capture a page with PageVault first", version))
	}
	for _, query := range []string{
		"SELECT id,url,title,hostname,first_seen,last_seen,latest_capture_id FROM pages LIMIT 0",
		"SELECT id,page_id,captured_at,content_hash,block_count,reason,payload_json FROM captures LIMIT 0",
		"SELECT id,page_id,block_hash,payload_json,search_text FROM blocks LIMIT 0",
	} {
		rows, err := db.Query(query)
		if err != nil {
			return fail(err)
		}
		if err := rows.Close(); err != nil {
			return fail(err)
		}
	}
	return &Store{DB: db, Path: p}, nil
}

// ListSitesContext returns bounded, paginated domain summaries for discovery.
func (s *Store) ListSitesContext(ctx context.Context, limit, offset int64) (map[string]any, error) {
	if _, err := Integer(limit, "limit", 1, 200); err != nil {
		return nil, err
	}
	if _, err := Integer(offset, "offset", 0, -1); err != nil {
		return nil, err
	}
	var total int64
	if err := s.DB.QueryRowContext(ctx, "SELECT count(DISTINCT hostname) FROM pages").Scan(&total); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT hostname,count(*) AS page_count FROM pages GROUP BY hostname ORDER BY hostname LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}
	items, err := readRows(rows)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		summary(item, map[string]int{"hostname": 256})
	}
	return map[string]any{"items": items, "total": total, "limit": limit, "offset": offset}, nil
}
