package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/akarazhev/pagevault/internal/archive"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPMissingDatabaseAndBadArgsStayOffStdout(t *testing.T) {
	for _, args := range [][]string{{"mcp"}, {"mcp", "unexpected"}, {"mcp", "--port", "8080"}, {"mcp", "chrome-extension://example/"}} {
		dir := t.TempDir()
		t.Setenv("PAGEVAULT_DB", filepath.Join(dir, "missing", "db.sqlite3"))
		var out, stderr bytes.Buffer
		if code := run(args, strings.NewReader(""), &out, &stderr, filepath.Join(dir, "pagevault-host")); code == 0 || out.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("%v: code=%d stdout=%q stderr=%q", args, code, out.String(), stderr.String())
		}
		if _, err := os.Stat(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
			t.Fatal("MCP created archive path", err)
		}
	}
}

func TestMCPCompiledStdioAndDatabaseSelection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dir := t.TempDir()
	binary := filepath.Join(dir, "pagevault-host")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-trimpath", "-buildvcs=false", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	paths := map[string]string{}
	for _, source := range []string{"config", "env", "explicit"} {
		path := filepath.Join(dir, source+" архив.sqlite3")
		w, err := archive.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Save(map[string]any{"schema_version": 1, "url": "https://" + source + ".example/", "title": source, "captured_at": "2026-10-28T12:00:00Z", "blocks": []any{}})
		w.Close()
		if err != nil {
			t.Fatal(err)
		}
		paths[source] = path
	}
	config, _ := json.Marshal(map[string]any{"schema_version": 1, "db_path": paths["config"]})
	if err := os.WriteFile(filepath.Join(dir, "pagevault.json"), config, 0600); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"config", "env", "explicit"} {
		t.Run(source, func(t *testing.T) {
			args := []string{"mcp"}
			t.Setenv("PAGEVAULT_DB", "")
			if source != "config" {
				t.Setenv("PAGEVAULT_DB", paths["env"])
			}
			if source == "explicit" {
				args = append(args, "--db", paths["explicit"])
			}
			cmd := exec.CommandContext(ctx, binary, args...)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			client := mcp.NewClient(&mcp.Implementation{Name: "stdio-test"}, nil)
			cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
			if err != nil {
				t.Fatalf("MCP initialize: %v stderr=%s", err, stderr.String())
			}
			r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_sites", Arguments: map[string]any{}})
			if err != nil || r.IsError {
				cs.Close()
				t.Fatal(r, err)
			}
			raw, _ := json.Marshal(r.StructuredContent)
			if !bytes.Contains(raw, []byte(source+".example")) {
				cs.Close()
				t.Fatalf("wrong database: %s", raw)
			}
			if err := cs.Close(); err != nil {
				t.Fatalf("MCP shutdown: %v", err)
			}
			if !cmd.ProcessState.Success() || stderr.Len() != 0 {
				t.Fatalf("exit=%v stderr=%s", cmd.ProcessState, stderr.String())
			}
		})
	}
}
