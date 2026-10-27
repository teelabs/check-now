package runner

import (
	"check-now/internal/protocol"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func script(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "plugin")
	if e := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0700); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestRunnerSeparatesStderrAndValidatesResult(t *testing.T) {
	p := script(t, "echo diagnostic >&2; echo '{\"protocol\":1,\"id\":\"x\",\"tool\":\"X\",\"status\":\"up_to_date\",\"checked_at\":\"2026-01-01T00:00:00Z\"}'")
	r, e := Runner{Timeout: time.Second}.Run(context.Background(), p, protocol.Request{Protocol: 1})
	if e != nil || r.ID != "x" {
		t.Fatalf("result=%+v err=%v", r, e)
	}
}

func TestRunnerSendsPluginIDInProtocolRequest(t *testing.T) {
	p := script(t, `request=$(cat); case "$request" in *'"id":"wrapped"'*) printf '{"protocol":1,"id":"wrapped","tool":"X","status":"up_to_date","checked_at":"2026-01-01T00:00:00Z"}' ;; *) exit 1 ;; esac`)
	r, e := (Runner{Timeout: time.Second}).Run(context.Background(), p, protocol.Request{Protocol: 1, ID: "wrapped"})
	if e != nil || r.ID != "wrapped" {
		t.Fatalf("result=%+v err=%v", r, e)
	}
}
func TestRunnerTimeout(t *testing.T) {
	p := script(t, "sleep 1")
	if _, e := (Runner{Timeout: 10 * time.Millisecond}).Run(context.Background(), p, protocol.Request{Protocol: 1}); e == nil {
		t.Fatal("expected timeout")
	}
}

func TestRunnerMetadataValidatesFolderIdentity(t *testing.T) {
	p := script(t, `printf '{"protocol":1,"id":"x","name":"X","description":"Checks X","runtime":"shell","capabilities":["refresh"]}'`)
	if _, err := (Runner{Timeout: time.Second}).Metadata(context.Background(), p, "different"); err == nil {
		t.Fatal("metadata ID mismatch accepted")
	}
}

func TestRunnerCurrentMode(t *testing.T) {
	p := script(t, `request=$(cat); case "$request" in *'"mode":"current"'*) printf '{"protocol":1,"id":"x","tool":"X","status":"up_to_date","current_version":"1.0.0","checked_at":"2026-01-01T00:00:00Z"}' ;; *) exit 1 ;; esac`)
	r, err := (Runner{Timeout: time.Second}).Run(context.Background(), p, protocol.Request{Protocol: 1, ID: "x", Mode: protocol.ModeCurrent})
	if err != nil || r.CurrentVersion != "1.0.0" {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}

func TestRunnerRejectsUnboundedPluginOutput(t *testing.T) {
	p := script(t, `dd if=/dev/zero bs=1048576 count=2 2>/dev/null`)
	if _, err := (Runner{Timeout: time.Second}).Run(context.Background(), p, protocol.Request{Protocol: 1, ID: "x", Mode: protocol.ModeCurrent}); err == nil {
		t.Fatal("oversized plugin output accepted")
	}
}

func TestRunnerRejectsRemoteFieldsInCurrentMode(t *testing.T) {
	p := script(t, `printf '{"protocol":1,"id":"x","tool":"X","status":"update_available","current_version":"1.0.0","target_version":"2.0.0","checked_at":"2026-01-01T00:00:00Z"}'`)
	if _, err := (Runner{Timeout: time.Second}).Run(context.Background(), p, protocol.Request{Protocol: 1, ID: "x", Mode: protocol.ModeCurrent}); err == nil {
		t.Fatal("current response with target fields accepted")
	}
}
