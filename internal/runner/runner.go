package runner

import (
	"bytes"
	"check-now/internal/protocol"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

type Runner struct{ Timeout time.Duration }

const maxPluginOutput = 1 << 20

var errPluginOutputTooLarge = errors.New("plugin output exceeds 1 MiB limit")

type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.max - b.buf.Len()
	if remaining <= 0 {
		return 0, errPluginOutputTooLarge
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		return remaining, errPluginOutputTooLarge
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string { return b.buf.String() }
func (b *limitedBuffer) Bytes() []byte  { return b.buf.Bytes() }

func (r Runner) Run(ctx context.Context, path string, req protocol.Request) (protocol.Result, error) {
	if req.Mode == "" {
		req.Mode = protocol.ModeRefresh
	}
	if err := protocol.ValidateRequest(req); err != nil {
		return protocol.Result{}, err
	}
	if r.Timeout <= 0 {
		r.Timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	b, _ := protocol.Encode(req)
	cmd := exec.CommandContext(ctx, path)
	configureProcessGroup(cmd)
	defer cleanupProcessGroup(cmd)
	cmd.Stdin = bytes.NewReader(append(b, '\n'))
	var out, diag limitedBuffer
	out.max, diag.max = maxPluginOutput, maxPluginOutput
	cmd.Stdout = &out
	cmd.Stderr = &diag
	err := cmd.Run()
	if ctx.Err() != nil {
		return protocol.Result{}, fmt.Errorf("plugin timeout: %w", ctx.Err())
	}
	if err != nil {
		return protocol.Result{}, fmt.Errorf("plugin failed: %w (%s)", err, diag.String())
	}
	res, e := protocol.DecodeResult(out.Bytes())
	if e != nil {
		return protocol.Result{}, fmt.Errorf("plugin output: %w", e)
	}
	if req.ID != "" && res.ID != req.ID {
		return protocol.Result{}, fmt.Errorf("plugin output: id %q does not match %q", res.ID, req.ID)
	}
	if req.Mode == protocol.ModeCurrent && (res.Status == "update_available" || res.TargetVersion != "" || res.Track != "" || res.SourceURL != "" || res.Argv != nil || len(res.Updates) != 0) {
		return protocol.Result{}, fmt.Errorf("current response contains target or remote fields")
	}
	return res, nil
}

// Health runs the adapter's local diagnostic mode and deliberately omits
// plugin stderr from returned errors so the doctor never retains diagnostics
// that may contain environment-specific data.
func (r Runner) Health(ctx context.Context, path, id string) (protocol.Health, error) {
	if r.Timeout <= 0 {
		r.Timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	b, _ := protocol.Encode(protocol.Request{Protocol: protocol.Version, ID: id, Mode: protocol.ModeHealth})
	cmd := exec.CommandContext(ctx, path)
	configureProcessGroup(cmd)
	defer cleanupProcessGroup(cmd)
	cmd.Stdin = bytes.NewReader(append(b, '\n'))
	var out limitedBuffer
	out.max = maxPluginOutput
	cmd.Stdout, cmd.Stderr = &out, io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return protocol.Health{}, fmt.Errorf("plugin health timeout")
		}
		return protocol.Health{}, fmt.Errorf("plugin health process failed")
	}
	h, err := protocol.DecodeHealth(out.Bytes())
	if err != nil {
		return protocol.Health{}, fmt.Errorf("plugin health output invalid")
	}
	if h.ID != id {
		return protocol.Health{}, fmt.Errorf("plugin health ID mismatch")
	}
	return h, nil
}

func (r Runner) Metadata(ctx context.Context, path, id string) (protocol.Metadata, error) {
	if r.Timeout <= 0 {
		r.Timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	b, _ := protocol.Encode(protocol.Request{Protocol: protocol.Version, ID: id, Mode: protocol.ModeMetadata})
	cmd := exec.CommandContext(ctx, path)
	configureProcessGroup(cmd)
	defer cleanupProcessGroup(cmd)
	cmd.Stdin = bytes.NewReader(append(b, '\n'))
	var out, diag limitedBuffer
	out.max, diag.max = maxPluginOutput, maxPluginOutput
	cmd.Stdout, cmd.Stderr = &out, &diag
	if err := cmd.Run(); err != nil {
		return protocol.Metadata{}, fmt.Errorf("plugin metadata failed: %w (%s)", err, diag.String())
	}
	m, err := protocol.DecodeMetadata(out.Bytes())
	if err != nil {
		return m, err
	}
	if m.ID != id {
		return m, fmt.Errorf("plugin metadata id %q does not match %q", m.ID, id)
	}
	if !hasCapability(m.Capabilities, protocol.ModeCurrent) || !hasCapability(m.Capabilities, protocol.ModeRefresh) || !hasCapability(m.Capabilities, protocol.ModeHealth) {
		return m, fmt.Errorf("plugin metadata must advertise current, refresh, and health capabilities")
	}
	return m, nil
}

func hasCapability(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// FailureOutcome maps adapter errors to a bounded value suitable for refresh
// history. Error details and plugin stderr are never copied into the result.
func FailureOutcome(err error) string {
	if err == nil {
		return "success"
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(err.Error(), "plugin timeout"):
		return "timeout"
	case strings.Contains(err.Error(), "plugin output"):
		return "protocol_failure"
	default:
		return "process_failure"
	}
}
