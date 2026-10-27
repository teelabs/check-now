package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"check-now/internal/scaffold"
)

func TestPromptPluginExplainsOptionalFields(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	p := scaffold.Options{Runtime: "shell"}
	promptPlugin(&p, new(string), bufio.NewReader(strings.NewReader("id\nName\nDescription\n\n\n\n\n")))
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if !bytes.Contains(out, []byte("optional")) || !bytes.Contains(out, []byte("local command")) || !bytes.Contains(out, []byte("source URL")) {
		t.Fatalf("wizard output lacks short field explanations: %s", out)
	}
}
