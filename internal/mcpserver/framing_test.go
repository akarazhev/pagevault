package mcpserver

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestLineReaderBoundariesAndSplitReads(t *testing.T) {
	for _, n := range []int{maxInputLine - 1, maxInputLine, maxInputLine + 1} {
		for _, newline := range []bool{false, true} {
			data := strings.Repeat("x", n)
			if newline {
				data = data[:n-1] + "\n"
			}
			reader := newLineReader(io.NopCloser(iotest.OneByteReader(strings.NewReader(data))))
			got, err := io.ReadAll(reader)
			if n <= maxInputLine && (err != nil || !bytes.Equal(got, []byte(data))) {
				t.Fatalf("size %d newline %v: %d %v", n, newline, len(got), err)
			}
			if n > maxInputLine && err == nil {
				t.Fatalf("accepted %d", n)
			}
		}
	}
}
