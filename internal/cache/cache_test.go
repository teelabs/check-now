package cache

import (
	"check-now/internal/protocol"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func result() protocol.Result {
	return protocol.Result{Protocol: 1, ID: "x", Tool: "X", Status: "up_to_date", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
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
	if err := s.Write(protocol.Result{Protocol: 1, ID: "bun", Tool: "Bun", Status: "unknown", CheckedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.ReadAny("node"); ok {
		t.Fatal("mismatched cache identity accepted")
	}
}
