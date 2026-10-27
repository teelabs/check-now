package cache

import (
	"check-now/internal/protocol"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Store struct {
	Dir   string
	Stale time.Duration
}

func New(dir string, stale time.Duration) Store { return Store{dir, stale} }
func (s Store) Path(id string) string           { return filepath.Join(s.Dir, id+".json") }
func (s Store) Read(id string) (protocol.Result, bool) {
	b, e := os.ReadFile(s.Path(id))
	if e != nil {
		return protocol.Result{}, false
	}
	r, e := protocol.DecodeResult(b)
	if e != nil {
		return r, false
	}
	t, e := time.Parse(time.RFC3339, r.CheckedAt)
	return r, e == nil && time.Since(t) <= s.Stale
}
func (s Store) ReadAny(id string) (protocol.Result, bool) {
	b, e := os.ReadFile(s.Path(id))
	if e != nil {
		return protocol.Result{}, false
	}
	r, e := protocol.DecodeResult(b)
	return r, e == nil && r.ID == id
}
func (s Store) Write(r protocol.Result) error {
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(r, "", "  ")
	tmp := s.Path(r.ID) + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.Path(r.ID)); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
func (s Store) Lock(name string, wait time.Duration) (func(), error) {
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return nil, err
	}
	p := filepath.Join(s.Dir, name+".lock")
	deadline := time.Now().Add(wait)
	for {
		f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e == nil {
			if _, writeErr := fmt.Fprintf(f, "%d\n", os.Getpid()); writeErr != nil {
				_ = f.Close()
				_ = os.Remove(p)
				return nil, writeErr
			}
			if closeErr := f.Close(); closeErr != nil {
				_ = os.Remove(p)
				return nil, closeErr
			}
			return func() { _ = os.Remove(p) }, nil
		}
		if !os.IsExist(e) {
			return nil, e
		}
		if staleLock(p, wait) {
			_ = os.Remove(p)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("lock timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func staleLock(path string, wait time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) < wait {
		return false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return true
	}
	err = syscall.Kill(pid, 0)
	return err != nil && err != syscall.EPERM
}
