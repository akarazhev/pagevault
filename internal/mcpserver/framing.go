package mcpserver

import (
	"bufio"
	"fmt"
	"io"
)

const maxInputLine = 64 * 1024

type lineReader struct {
	source   io.ReadCloser
	buffered *bufio.Reader
	pending  []byte
	err      error
}

func newLineReader(r io.ReadCloser) *lineReader {
	return &lineReader{source: r, buffered: bufio.NewReader(r)}
}
func (r *lineReader) Close() error { return r.source.Close() }
func (r *lineReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.pending) == 0 && r.err == nil {
		for {
			part, err := r.buffered.ReadSlice('\n')
			if len(r.pending)+len(part) > maxInputLine {
				r.pending = nil
				r.err = fmt.Errorf("input frame exceeds %d bytes", maxInputLine)
				break
			}
			r.pending = append(r.pending, part...)
			if err == bufio.ErrBufferFull {
				continue
			}
			if err != nil {
				r.err = err
			}
			break
		}
	}
	if len(r.pending) > 0 {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		return n, nil
	}
	return 0, r.err
}
