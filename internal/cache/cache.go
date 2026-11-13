package cache

import (
	"check-now/internal/protocol"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// Cache writes are pretty-printed and may be larger than the adapter's 1 MiB
// response limit, so allow headroom while keeping reads bounded.
const maxCacheRecordSize = 2 << 20

var errCacheRecordTooLarge = errors.New("cache record exceeds 2 MiB")
var errCacheRecordNotRegular = errors.New("cache record is not a regular file")

type Control struct {
	Dismissed   bool   `json:"dismissed"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type RefreshRecord struct {
	CheckedAt string `json:"checked_at"`
	Outcome   string `json:"outcome"`
}

type RecordState string

const (
	RecordMissing    RecordState = "missing"
	RecordValid      RecordState = "valid"
	RecordLegacy     RecordState = "legacy"
	RecordCorrupt    RecordState = "corrupt"
	RecordUnreadable RecordState = "unreadable"
)

func New(dir string, stale time.Duration) Store { return Store{dir, stale} }
func (s Store) Path(id string) string           { return filepath.Join(s.Dir, id+".json") }
func (s Store) TargetPath(id string) string     { return filepath.Join(s.Dir, "targets", id+".json") }
func (s Store) CurrentPath(id string) string    { return filepath.Join(s.Dir, "current", id+".json") }
func (s Store) ControlPath(id string) string    { return filepath.Join(s.Dir, "control", id+".json") }
func (s Store) RefreshPath(id string) string    { return filepath.Join(s.Dir, "refresh", id+".json") }

func readResult(path, id string) (protocol.Result, bool) {
	b, err := readCacheRecord(path)
	if err != nil {
		return protocol.Result{}, false
	}
	r, err := protocol.DecodeCachedResult(b)
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

// InspectTarget reads cache state without migrating a legacy combined record.
func (s Store) InspectTarget(id string) (protocol.Result, RecordState) {
	r, state := inspectResult(s.TargetPath(id), id)
	if state != RecordMissing {
		return r, state
	}
	r, state = inspectResult(s.Path(id), id)
	if state == RecordValid {
		return r, RecordLegacy
	}
	return r, state
}

func (s Store) InspectCurrent(id string) (protocol.Result, RecordState) {
	return inspectResult(s.CurrentPath(id), id)
}

func inspectResult(path, id string) (protocol.Result, RecordState) {
	b, err := readCacheRecord(path)
	if err != nil {
		if invalidCacheRecordError(err) {
			return protocol.Result{}, RecordCorrupt
		}
		if os.IsNotExist(err) {
			return protocol.Result{}, RecordMissing
		}
		return protocol.Result{}, RecordUnreadable
	}
	r, err := protocol.DecodeCachedResult(b)
	if err != nil || (id != "" && r.ID != id) {
		return protocol.Result{}, RecordCorrupt
	}
	return r, RecordValid
}

func readCacheRecord(path string) ([]byte, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errCacheRecordNotRegular
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCacheRecordSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCacheRecordSize {
		return nil, errCacheRecordTooLarge
	}
	return data, nil
}

func invalidCacheRecordError(err error) bool {
	return errors.Is(err, errCacheRecordTooLarge) || errors.Is(err, errCacheRecordNotRegular)
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
	b, err := readCacheRecord(s.ControlPath(id))
	if err != nil {
		return Control{}
	}
	var c Control
	if json.Unmarshal(b, &c) != nil {
		return Control{}
	}
	return c
}

func (s Store) InspectControl(id string) (Control, RecordState) {
	b, err := readCacheRecord(s.ControlPath(id))
	if err != nil {
		if invalidCacheRecordError(err) {
			return Control{}, RecordCorrupt
		}
		if os.IsNotExist(err) {
			return Control{}, RecordMissing
		}
		return Control{}, RecordUnreadable
	}
	var control Control
	if err := json.Unmarshal(b, &control); err != nil {
		return Control{}, RecordCorrupt
	}
	return control, RecordValid
}

var refreshOutcomes = map[string]bool{
	"success": true, "unknown": true, "timeout": true,
	"process_failure": true, "protocol_failure": true, "cache_failure": true,
}

func (s Store) ReadRefresh(id string) (RefreshRecord, bool) {
	record, state := s.InspectRefresh(id)
	return record, state == RecordValid
}

func (s Store) InspectRefresh(id string) (RefreshRecord, RecordState) {
	b, err := readCacheRecord(s.RefreshPath(id))
	if err != nil {
		if invalidCacheRecordError(err) {
			return RefreshRecord{}, RecordCorrupt
		}
		if os.IsNotExist(err) {
			return RefreshRecord{}, RecordMissing
		}
		return RefreshRecord{}, RecordUnreadable
	}
	var record RefreshRecord
	if json.Unmarshal(b, &record) != nil || !refreshOutcomes[record.Outcome] {
		return RefreshRecord{}, RecordCorrupt
	}
	if _, err := time.Parse(time.RFC3339, record.CheckedAt); err != nil {
		return RefreshRecord{}, RecordCorrupt
	}
	return record, RecordValid
}

func (s Store) WriteRefresh(id string, record RefreshRecord) error {
	if id == "" || !refreshOutcomes[record.Outcome] {
		return fmt.Errorf("invalid refresh outcome")
	}
	if _, err := time.Parse(time.RFC3339, record.CheckedAt); err != nil {
		return fmt.Errorf("invalid refresh timestamp")
	}
	if err := os.MkdirAll(filepath.Dir(s.RefreshPath(id)), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	path := s.RefreshPath(id)
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
