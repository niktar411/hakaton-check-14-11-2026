//go:build windows

package worker

import "os/exec"

func configureProcess(*exec.Cmd) {}

func killProcess(command *exec.Cmd) {
	if command.Process != nil {
		_ = command.Process.Kill()
	}
}
