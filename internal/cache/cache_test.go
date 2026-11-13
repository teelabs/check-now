package cache

import (
	"check-now/internal/protocol"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func result() protocol.Result {
	return protocol.Result{Protocol: protocol.Version, ID: "x", Tool: "X", Status: "up_to_date", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
}

func TestLockRecoversDeadStaleOwner(t *testing.T) {
	s := New(t.TempDir(), time.Hour)
	lock := filepath.Join(s.Dir, "refresh.lock")
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, []byte("2147483647\n"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	unlock, err := s.Lock("refresh", 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}
func TestAtomicWriteAndStaleRead(t *testing.T) {
	s := New(t.TempDir(), time.Hour)
	r := result()
	if e := s.Write(r); e != nil {
		t.Fatal(e)
	}
	if _, ok := s.Read("x"); !ok {
		t.Fatal("fresh result not read")
	}
	r.CheckedAt = time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	if e := s.Write(r); e != nil {
		t.Fatal(e)
	}
	if _, ok := s.Read("x"); ok {
		t.Fatal("stale result reported fresh")
	}
	if _, ok := s.ReadAny("x"); !ok {
		t.Fatal("last valid result lost")
	}
}

func TestReadAnyRejectsFilenameIdentityMismatch(t *testing.T) {
	s := New(t.TempDir(), time.Hour)
	if err := s.Write(protocol.Result{Protocol: protocol.Version, ID: "bun", Tool: "Bun", Status: "unknown", CheckedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.ReadAny("node"); ok {
		t.Fatal("mismatched cache identity accepted")
	}
}

func TestTargetAndCurrentCachesAreIndependent(t *testing.T) {
	s := New(t.TempDir(), time.Hour)
	target := protocol.Result{Protocol: protocol.Version, ID: "x", Tool: "X", Status: "update_available", TargetVersion: "2.0.0", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	current := protocol.Result{Protocol: protocol.Version, ID: "x", Tool: "X", Status: "up_to_date", CurrentVersion: "1.0.0", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := s.WriteTarget(target); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteCurrent(current); err != nil {
		t.Fatal(err)
	}
	gotTarget, ok := s.ReadTarget("x")
	if !ok || gotTarget.TargetVersion != "2.0.0" {
		t.Fatalf("target=%+v ok=%v", gotTarget, ok)
	}
	gotCurrent, ok := s.ReadCurrent("x")
	if !ok || gotCurrent.CurrentVersion != "1.0.0" {
		t.Fatalf("current=%+v ok=%v", gotCurrent, ok)
	}
}

func TestInspectTargetDoesNotMigrateLegacyRecord(t *testing.T) {
	s := New(t.TempDir(), time.Hour)
	if err := os.WriteFile(s.Path("x"), []byte(`{"protocol":2,"id":"x","tool":"X","status":"unknown","checked_at":"2026-01-01T00:00:00Z"}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, state := s.InspectTarget("x")
	if state != RecordLegacy || got.ID != "x" {
		t.Fatalf("result=%+v state=%s", got, state)
	}
	if _, err := os.Stat(s.TargetPath("x")); !os.IsNotExist(err) {
		t.Fatalf("inspection migrated legacy record: stat err=%v", err)
	}
}

func TestCacheReadsPreviousProtocolTargetForLastKnownGoodPreservation(t *testing.T) {
	s := New(t.TempDir(), time.Hour)
	if err := os.MkdirAll(filepath.Dir(s.TargetPath("x")), 0700); err != nil {
		t.Fatal(err)
	}
	previous := `{"protocol":1,"id":"x","tool":"X","status":"update_available","target_version":"2.0.0","checked_at":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(s.TargetPath("x"), []byte(previous), 0600); err != nil {
		t.Fatal(err)
	}
	got, ok := s.ReadTarget("x")
	if !ok || got.Protocol != protocol.Version || got.TargetVersion != "2.0.0" {
		t.Fatalf("target=%+v ok=%v", got, ok)
	}
}

func TestRefreshHistoryRejectsUnboundedOutcomeText(t *testing.T) {
	s := New(t.TempDir(), time.Hour)
	err := s.WriteRefresh("x", RefreshRecord{CheckedAt: time.Now().UTC().Format(time.RFC3339), Outcome: "private plugin diagnostic"})
	if err == nil {
		t.Fatal("unbounded refresh outcome accepted")
	}
}

func TestInspectRejectsOversizedCacheRecords(t *testing.T) {
	s := New(t.TempDir(), time.Hour)
	padding := strings.Repeat("x", 2<<20)
	records := []struct {
		path string
		body string
	}{
		{s.TargetPath("x"), `{"protocol":2,"id":"x","tool":"X","status":"unknown","checked_at":"2026-01-01T00:00:00Z","explanation":"` + padding + `"}`},
		{s.CurrentPath("x"), `{"protocol":2,"id":"x","tool":"X","status":"unknown","checked_at":"2026-01-01T00:00:00Z","explanation":"` + padding + `"}`},
		{s.ControlPath("x"), `{"dismissed":false,"padding":"` + padding + `"}`},
		{s.RefreshPath("x"), `{"checked_at":"2026-01-01T00:00:00Z","outcome":"success","padding":"` + padding + `"}`},
	}
	for _, record := range records {
		path := record.path
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(record.body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, state := s.InspectTarget("x"); state != RecordCorrupt {
		t.Errorf("oversized target state = %s; want corrupt", state)
	}
	if _, state := s.InspectCurrent("x"); state != RecordCorrupt {
		t.Errorf("oversized current state = %s; want corrupt", state)
	}
	if _, state := s.InspectControl("x"); state != RecordCorrupt {
		t.Errorf("oversized control state = %s; want corrupt", state)
	}
	if _, state := s.InspectRefresh("x"); state != RecordCorrupt {
		t.Errorf("oversized refresh state = %s; want corrupt", state)
	}
}

func TestInspectRejectsFIFOWithoutBlocking(t *testing.T) {
	s := New(t.TempDir(), time.Hour)
	if err := os.MkdirAll(filepath.Dir(s.TargetPath("x")), 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(s.TargetPath("x"), 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan RecordState, 1)
	go func() {
		_, state := s.InspectTarget("x")
		done <- state
	}()
	select {
	case state := <-done:
		if state != RecordCorrupt {
			t.Fatalf("FIFO target state = %s; want corrupt", state)
		}
	case <-time.After(time.Second):
		t.Fatal("inspecting a FIFO cache record blocked")
	}
}
