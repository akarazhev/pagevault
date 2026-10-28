// Package native implements the Chromium Native Messaging protocol.
package native

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"pagevault.local/pagevault/internal/archive"
	"pagevault.local/pagevault/internal/buildinfo"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxIncoming = 32 * 1024 * 1024
const MaxOutgoing = 1024 * 1024

type host struct {
	store *archive.Store
	id    string
}

func newHost(s *archive.Store) *host {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return &host{s, hex.EncodeToString(b[:])}
}
func errorReply(id any, code, message string) map[string]any {
	return map[string]any{"id": id, "ok": false, "error": map[string]any{"code": code, "message": message}}
}
func (h *host) info() (map[string]any, error) {
	var sqliteVersion string
	if e := h.store.DB.QueryRow("SELECT sqlite_version()").Scan(&sqliteVersion); e != nil {
		return nil, e
	}
	return map[string]any{"version": buildinfo.Version, "schema_version": 1, "db_path": h.store.Path, "go_version": runtime.Version(), "sqlite_version": sqliteVersion, "host_id": h.id, "process_id": os.Getpid(), "db_connections": h.store.DB.Stats().OpenConnections}, nil
}
func (h *host) handle(value any) map[string]any {
	var requestID any
	request, ok := value.(map[string]any)
	if ok {
		requestID = request["id"]
	}
	id, valid := requestID.(string)
	if !ok || !valid || id == "" || utf8.RuneCountInString(id) > 256 {
		return errorReply(nil, "invalid_request", "Request needs a string id of 1..256 characters")
	}
	ty, ok := request["type"].(string)
	if !ok {
		return errorReply(id, "invalid_request", "Request needs a string type")
	}
	var data any
	var e error
	integer := func(name string, def, min, max int64) (int64, error) {
		v, ok := request[name]
		if !ok {
			v = def
		}
		return archive.Integer(v, name, min, max)
	}
	switch ty {
	case "ping":
		data, e = h.info()
	case "capture", "capture.save":
		value, ok := request["capture"]
		if !ok {
			value = request["payload"]
		}
		c, _ := value.(map[string]any)
		data, e = h.store.Save(c)
	case "pages.list":
		query, domain := "", ""
		if v, ok := request["query"]; ok {
			var valid bool
			query, valid = v.(string)
			if !valid {
				e = &archive.ValidationError{Message: "query and domain must be text"}
			}
		}
		if v, ok := request["domain"]; ok {
			var valid bool
			domain, valid = v.(string)
			if !valid {
				e = &archive.ValidationError{Message: "query and domain must be text"}
			}
		}
		if e == nil {
			var limit, offset int64
			limit, e = integer("limit", 50, 1, 200)
			if e == nil {
				offset, e = integer("offset", 0, 0, -1)
			}
			if e == nil {
				data, e = h.store.ListPages(query, domain, limit, offset)
			}
		}
	case "pages.get":
		var pageID, limit, offset int64
		pageID, e = integer("page_id", 0, 1, -1)
		if e == nil {
			limit, e = integer("limit", 100, 1, 500)
		}
		if e == nil {
			offset, e = integer("offset", 0, 0, -1)
		}
		if e == nil {
			data, e = h.store.GetPage(pageID, limit, offset)
		}
	case "captures.get":
		var captureID int64
		captureID, e = integer("capture_id", 0, 1, -1)
		if e == nil {
			var raw string
			raw, e = h.store.CaptureJSON(captureID)
			if e == nil {
				data, e = chunk(raw, request, id)
			}
		}
	case "pages.export":
		var pageID int64
		pageID, e = integer("page_id", 0, 1, -1)
		if e == nil {
			var exported map[string]any
			exported, e = h.store.Export(pageID)
			if e == nil {
				var raw string
				raw, e = archive.Canonical(exported)
				if e == nil {
					data, e = chunk(raw, request, id)
				}
			}
		}
	case "pages.domains":
		var limit, offset int64
		limit, e = integer("limit", 200, 1, 200)
		if e == nil {
			offset, e = integer("offset", 0, 0, -1)
		}
		if e == nil {
			data, e = h.store.DomainsPage(limit, offset)
		}
	case "stats":
		data, e = h.store.Stats()
	case "pages.delete":
		var pageID int64
		pageID, e = integer("page_id", 0, 1, -1)
		if e == nil {
			data, e = h.store.Delete(pageID)
		}
	default:
		return errorReply(id, "unknown_command", "Unknown command")
	}
	if e != nil {
		var validation *archive.ValidationError
		var missing *archive.NotFoundError
		switch {
		case errors.As(e, &validation):
			return errorReply(id, "invalid_request", e.Error())
		case errors.As(e, &missing):
			return errorReply(id, "not_found", e.Error())
		default:
			return errorReply(id, "storage_error", e.Error())
		}
	}
	response := map[string]any{"id": id, "ok": true, "data": data}
	raw, e := archive.Canonical(response)
	if e != nil {
		return errorReply(id, "storage_error", e.Error())
	}
	if len(raw) >= MaxOutgoing {
		return errorReply(id, "response_too_large", "Response exceeds Native Messaging limit; reduce limit or use chunked export")
	}
	return response
}
func chunk(content string, request map[string]any, id string) (map[string]any, error) {
	offsetValue, ok := request["offset"]
	if !ok {
		offsetValue = int64(0)
	}
	offset, e := archive.Integer(offsetValue, "offset", 0, -1)
	if e != nil {
		return nil, e
	}
	sizeValue, ok := request["chunk_size"]
	if !ok {
		sizeValue = int64(120000)
	}
	size, e := archive.Integer(sizeValue, "chunk_size", 1, 160000)
	if e != nil {
		return nil, e
	}
	r := []rune(content)
	total := int64(len(r))
	if offset > total {
		return nil, &archive.ValidationError{Message: "offset exceeds total_chars"}
	}
	end := min(offset+size, total)
	digest := archive.Hash(content)
	for {
		data := map[string]any{"json_chunk": string(r[offset:end]), "offset": offset, "next_offset": end, "total_chars": total, "done": end == total, "document_hash": digest}
		raw, e := archive.Canonical(map[string]any{"id": id, "ok": true, "data": data})
		if e != nil {
			return nil, e
		}
		if len(raw) < MaxOutgoing {
			return data, nil
		}
		end = offset + (end-offset)/2
		if end == offset {
			return nil, &archive.ValidationError{Message: "Cannot fit response in Native Messaging frame"}
		}
	}
}
func writeFrame(out io.Writer, value any) error {
	raw, e := archive.Canonical(value)
	if e != nil {
		return e
	}
	if len(raw) >= MaxOutgoing {
		raw, _ = archive.Canonical(errorReply(nil, "response_too_large", "Response exceeds frame limit"))
	}
	var header [4]byte
	binary.NativeEndian.PutUint32(header[:], uint32(len(raw)))
	if e = writeAll(out, header[:]); e != nil {
		return e
	}
	if e = writeAll(out, []byte(raw)); e != nil {
		return e
	}
	if f, ok := out.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, e := w.Write(b)
		if e != nil {
			return e
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}

// Serve opens one retained SQLite connection before processing any frames and
// closes it only on EOF or a fatal transport error. Recoverable JSON/request
// errors each produce one response and leave the session intact.
func Serve(dbPath string, in io.Reader, out io.Writer) error {
	s, e := archive.Open(dbPath)
	if e != nil {
		if writeErr := writeFrame(out, errorReply(nil, "storage_error", e.Error())); writeErr != nil {
			return writeErr
		}
		return e
	}
	defer s.Close()
	h := newHost(s)
	fatal := func(code, message string) error {
		if e := writeFrame(out, errorReply(nil, code, message)); e != nil {
			return e
		}
		return errors.New(message)
	}
	for {
		var header [4]byte
		n, e := io.ReadFull(in, header[:])
		if e == io.EOF && n == 0 {
			return nil
		}
		if e != nil {
			return fatal("invalid_frame", "Truncated length prefix")
		}
		size := binary.NativeEndian.Uint32(header[:])
		if size > MaxIncoming {
			return fatal("frame_too_large", "Incoming frame exceeds 32 MiB")
		}
		raw := make([]byte, size)
		if _, e = io.ReadFull(in, raw); e != nil {
			return fatal("invalid_frame", "Truncated payload")
		}
		if !utf8.Valid(raw) {
			if e = writeFrame(out, errorReply(nil, "invalid_json", "Payload must be UTF-8 JSON")); e != nil {
				return e
			}
			continue
		}
		d := json.NewDecoder(strings.NewReader(string(raw)))
		d.UseNumber()
		var v any
		e = d.Decode(&v)
		if e == nil {
			var extra any
			if second := d.Decode(&extra); second != io.EOF {
				e = fmt.Errorf("trailing JSON")
			}
		}
		if e != nil {
			if e = writeFrame(out, errorReply(nil, "invalid_json", "Payload must be UTF-8 JSON")); e != nil {
				return e
			}
			continue
		}
		if !validSurrogates(string(raw)) {
			var id any
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(raw, &fields)
			if r, ok := v.(map[string]any); ok {
				if s, ok := r["id"].(string); ok && validSurrogates(string(fields["id"])) {
					id = s
				}
			}
			if e = writeFrame(out, errorReply(id, "invalid_request", "Text must contain valid Unicode")); e != nil {
				return e
			}
			continue
		}
		if e = writeFrame(out, h.handle(v)); e != nil {
			return e
		}
	}
}

// encoding/json replaces lone UTF-16 surrogates with U+FFFD. Reject them before
// accepting a capture, while preserving legitimate U+FFFD source characters.
func validSurrogates(raw string) bool {
	in := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			in = !in
			continue
		}
		if !in || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		n, e := strconv.ParseUint(raw[i+1:i+5], 16, 16)
		if e != nil {
			return false
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1:i+3] != `\u` {
				return false
			}
			m, e := strconv.ParseUint(raw[i+3:i+7], 16, 16)
			if e != nil || m < 0xdc00 || m > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
func Diagnose(dbPath string) (map[string]any, error) {
	s, e := archive.Open(dbPath)
	if e != nil {
		return nil, e
	}
	defer s.Close()
	h := newHost(s)
	info, e := h.info()
	if e != nil {
		return nil, e
	}
	stats, e := s.Stats()
	if e != nil {
		return nil, e
	}
	for k, v := range stats {
		info[k] = v
	}
	info["ok"] = true
	return info, nil
}
