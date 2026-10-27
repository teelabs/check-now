package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const Version = 1

const (
	ModeMetadata = "metadata"
	ModeCurrent  = "current"
	ModeRefresh  = "refresh"
)

type Request struct {
	Protocol  int               `json:"protocol"`
	ID        string            `json:"id,omitempty"`
	Mode      string            `json:"mode,omitempty"`
	Timestamp string            `json:"timestamp,omitempty"`
	Config    map[string]string `json:"config,omitempty"`
	Prior     *Result           `json:"prior,omitempty"`
}
type Metadata struct {
	Protocol     int      `json:"protocol"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Tool         string   `json:"tool,omitempty"`
	SourceURL    string   `json:"source_url,omitempty"`
	Runtime      string   `json:"runtime"`
	Manager      string   `json:"manager,omitempty"`
	Capabilities []string `json:"capabilities"`
}
type Update struct {
	Scope       string   `json:"scope"`
	Track       string   `json:"track"`
	Version     string   `json:"version"`
	SourceURL   string   `json:"source_url,omitempty"`
	Explanation string   `json:"explanation,omitempty"`
	Argv        []string `json:"argv,omitempty"`
}
type Result struct {
	Protocol         int      `json:"protocol"`
	ID               string   `json:"id"`
	Status           string   `json:"status"`
	Tool             string   `json:"tool"`
	InstallationKind string   `json:"installation_kind,omitempty"`
	Manager          string   `json:"manager,omitempty"`
	CurrentVersion   string   `json:"current_version,omitempty"`
	TargetVersion    string   `json:"target_version,omitempty"`
	Track            string   `json:"track,omitempty"`
	SourceURL        string   `json:"source_url,omitempty"`
	Explanation      string   `json:"explanation,omitempty"`
	Argv             []string `json:"argv,omitempty"`
	Updates          []Update `json:"updates,omitempty"`
	CheckedAt        string   `json:"checked_at"`
}

var statuses = map[string]bool{"not_installed": true, "up_to_date": true, "update_available": true, "unknown": true}

var modes = map[string]bool{ModeMetadata: true, ModeCurrent: true, ModeRefresh: true}

func ValidateRequest(r Request) error {
	if r.Protocol != Version {
		return fmt.Errorf("unsupported protocol %d", r.Protocol)
	}
	if !modes[r.Mode] {
		return fmt.Errorf("unsupported mode %q", r.Mode)
	}
	return nil
}

func DecodeResult(data []byte) (Result, error) {
	var r Result
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return r, fmt.Errorf("invalid result JSON: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return r, fmt.Errorf("invalid result JSON: multiple values")
	}
	if r.Protocol != Version {
		return r, fmt.Errorf("unsupported protocol %d", r.Protocol)
	}
	if r.ID == "" || r.Tool == "" || r.CheckedAt == "" || !statuses[r.Status] {
		return r, fmt.Errorf("missing or invalid result fields")
	}
	if _, err := time.Parse(time.RFC3339, r.CheckedAt); err != nil {
		return r, fmt.Errorf("invalid checked_at: %w", err)
	}
	for _, u := range r.Updates {
		if u.Scope == "" || u.Track == "" || u.Version == "" {
			return r, fmt.Errorf("invalid update choice")
		}
	}
	return r, nil
}
func DecodeMetadata(data []byte) (Metadata, error) {
	var m Metadata
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, fmt.Errorf("invalid metadata JSON: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return m, fmt.Errorf("invalid metadata JSON: multiple values")
	}
	if m.Protocol != Version || m.ID == "" || m.Name == "" || m.Description == "" || m.Runtime == "" || len(m.Capabilities) == 0 {
		return m, fmt.Errorf("missing or invalid metadata fields")
	}
	return m, nil
}

func Encode(v any) ([]byte, error) { return json.Marshal(v) }
func QuoteArgv(argv []string) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = "'" + strings.ReplaceAll(a, "'", "'\\''") + "'"
		if a != "" && safe(a) {
			out[i] = a
		}
	}
	return strings.Join(out, " ")
}
func safe(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-._/:@%+", r)) {
			return false
		}
	}
	return s != ""
}
