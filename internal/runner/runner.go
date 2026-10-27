package runner

import (
	"bytes"
	"check-now/internal/protocol"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type Runner struct{ Timeout time.Duration }

func (r Runner) Run(ctx context.Context, path string, req protocol.Request) (protocol.Result, error) {
	if r.Timeout <= 0 {
		r.Timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	b, _ := protocol.Encode(req)
	cmd := exec.CommandContext(ctx, path)
	cmd.Stdin = bytes.NewReader(append(b, '\n'))
	var out, diag bytes.Buffer
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
	return res, nil
}

func (r Runner) Metadata(ctx context.Context, path, id string) (protocol.Metadata, error) {
	if r.Timeout <= 0 {
		r.Timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	b, _ := protocol.Encode(protocol.Request{Protocol: protocol.Version, ID: id, Mode: "metadata"})
	cmd := exec.CommandContext(ctx, path)
	cmd.Stdin = bytes.NewReader(append(b, '\n'))
	var out, diag bytes.Buffer
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
	return m, nil
}
