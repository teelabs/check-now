//go:build !darwin && !linux

package runner

import "os/exec"

func configureProcessGroup(cmd *exec.Cmd) {}
func cleanupProcessGroup(cmd *exec.Cmd)   {}
