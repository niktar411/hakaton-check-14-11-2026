package worker

import (
	"bytes"
	"errors"
	"os/exec"
	"sync"
	"time"
)

// ProcessResult is the bounded output of one fixed command.
type ProcessResult struct {
	ReturnCode int
	Output     string
	Truncated  bool
}

// ProcessOptions controls one fixed command.
type ProcessOptions struct {
	Timeout     time.Duration
	OutputLimit int
	Dir         string
	Env         []string
	Input       string
}

type boundedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if len(data) <= remaining {
			b.buffer.Write(data)
		} else {
			b.buffer.Write(data[:remaining])
			b.truncated = true
		}
	} else if len(data) > 0 {
		b.truncated = true
	}
	// Always report the full write so the child never blocks on a closed pipe.
	return len(data), nil
}

// RunBounded executes a fixed argument vector with a hard timeout and bounded
// combined output. It never invokes a shell.
func RunBounded(args []string, options ProcessOptions) (ProcessResult, error) {
	if len(args) == 0 {
		return ProcessResult{}, workerError("invalid_command", 400)
	}
	for _, arg := range args {
		if arg == "" && args[0] != arg {
			continue
		}
		for _, r := range arg {
			if r == 0 {
				return ProcessResult{}, workerError("invalid_command", 400)
			}
		}
	}
	if options.OutputLimit <= 0 {
		options.OutputLimit = 64 * 1024
	}
	if options.Timeout <= 0 {
		options.Timeout = 30 * time.Second
	}
	buffer := &boundedBuffer{limit: options.OutputLimit}
	command := exec.Command(args[0], args[1:]...)
	command.Dir = options.Dir
	if options.Env != nil {
		command.Env = options.Env
	}
	if options.Input != "" {
		command.Stdin = bytes.NewBufferString(options.Input)
	}
	command.Stdout = buffer
	command.Stderr = buffer
	configureProcess(command)
	if err := command.Start(); err != nil {
		return ProcessResult{}, workerError("process_start_failed", 503)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	timer := time.NewTimer(options.Timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		result := ProcessResult{Output: buffer.buffer.String(), Truncated: buffer.truncated}
		if command.ProcessState != nil {
			result.ReturnCode = command.ProcessState.ExitCode()
		}
		if err != nil {
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) {
				return ProcessResult{}, workerError("process_start_failed", 503)
			}
		}
		return result, nil
	case <-timer.C:
		killProcess(command)
		<-done
		return ProcessResult{}, workerError("operation_timeout", 504)
	}
}

// Run executes a command and requires a zero exit status.
func Run(args []string, options ProcessOptions) (ProcessResult, error) {
	result, err := RunBounded(args, options)
	if err != nil {
		return ProcessResult{}, err
	}
	if result.ReturnCode != 0 {
		return ProcessResult{}, workerError("command_failed", 503)
	}
	return result, nil
}
