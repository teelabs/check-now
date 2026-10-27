package cache

import (
	"check-now/internal/protocol"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

type Store struct {
	Dir   string
	Stale time.Duration
}
type Control struct {
	Dismissed   bool   `json:"dismissed"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

func New(dir string, stale time.Duration) Store { return Store{dir, stale} }
func (s Store) Path(id string) string           { return filepath.Join(s.Dir, id+".json") }
func (s Store) TargetPath(id string) string     { return filepath.Join(s.Dir, "targets", id+".json") }
func (s Store) CurrentPath(id string) string    { return filepath.Join(s.Dir, "current", id+".json") }
func (s Store) ControlPath(id string) string    { return filepath.Join(s.Dir, "control", id+".json") }

func readResult(path, id string) (protocol.Result, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return protocol.Result{}, false
	}
	r, err := protocol.DecodeResult(b)
	return r, err == nil && (id == "" || r.ID == id)
}
func (s Store) ReadTarget(id string) (protocol.Result, bool) {
	if r, ok := readResult(s.TargetPath(id), id); ok {
		return r, true
	}
	if r, ok := readResult(s.Path(id), id); ok {
		// Migrate the legacy combined entry as a target with no dismissal state.
		_ = s.WriteTarget(r)
		return r, true
	}
	return protocol.Result{}, false
}
func (s Store) ReadCurrent(id string) (protocol.Result, bool) {
	return readResult(s.CurrentPath(id), id)
}
func (s Store) ReadTargetFresh(id string) (protocol.Result, bool) {
	r, ok := s.ReadTarget(id)
	if !ok {
		return r, false
	}
	t, err := time.Parse(time.RFC3339, r.CheckedAt)
	return r, err == nil && time.Since(t) <= s.Stale
}
func (s Store) Read(id string) (protocol.Result, bool)    { return s.ReadTargetFresh(id) }
func (s Store) ReadAny(id string) (protocol.Result, bool) { return s.ReadTarget(id) }

func (s Store) write(path string, r protocol.Result) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
func (s Store) WriteTarget(r protocol.Result) error  { return s.write(s.TargetPath(r.ID), r) }
func (s Store) WriteCurrent(r protocol.Result) error { return s.write(s.CurrentPath(r.ID), r) }
func (s Store) Write(r protocol.Result) error        { return s.WriteTarget(r) }

func (s Store) ReadControl(id string) Control {
	b, err := os.ReadFile(s.ControlPath(id))
	if err != nil {
		return Control{}
	}
	var c Control
	if json.Unmarshal(b, &c) != nil {
		return Control{}
	}
	return c
}
func (s Store) WriteControl(id string, c Control) error {
	if err := os.MkdirAll(filepath.Dir(s.ControlPath(id)), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := s.ControlPath(id)
	tmp := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
func Fingerprint(r protocol.Result) string {
	copy := r
	copy.CheckedAt = ""
	copy.CurrentVersion = ""
	b, _ := json.Marshal(copy)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s Store) Lock(name string, wait time.Duration) (func(), error) {
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return nil, err
	}
	p := filepath.Join(s.Dir, name+".lock")
	deadline := time.Now().Add(wait)
	for {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			if err := f.Truncate(0); err != nil {
				_ = f.Close()
				return nil, err
			}
			if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
				_ = f.Close()
				return nil, err
			}
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		_ = f.Close()
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("lock timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
