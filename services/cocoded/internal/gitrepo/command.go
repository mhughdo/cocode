package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
)

const defaultOutputLimit int64 = 1 << 20

var errOutputLimit = errors.New("git command output exceeded limit")

type Runner struct {
	GitPath     string
	Timeout     time.Duration
	OutputLimit int64
}

type CommandResult struct {
	Stdout string
	Stderr string
}

func DefaultRunner() Runner {
	return Runner{
		Timeout:     gitTimeout,
		OutputLimit: defaultOutputLimit,
	}
}

func (r Runner) Run(ctx context.Context, cwd string, args ...string) (CommandResult, error) {
	if strings.TrimSpace(cwd) == "" {
		return CommandResult{}, apperror.InvalidRequest("git command cwd is required")
	}
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return CommandResult{}, apperror.InvalidRequest("git command cwd is invalid")
	}
	stat, err := os.Stat(absCWD)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CommandResult{}, apperror.InvalidRequest("git command cwd does not exist")
		}
		return CommandResult{}, apperror.InvalidRequest("git command cwd cannot be inspected")
	}
	if !stat.IsDir() {
		return CommandResult{}, apperror.InvalidRequest("git command cwd must be a directory")
	}

	gitPath := r.GitPath
	if gitPath == "" {
		gitPath, err = exec.LookPath("git")
		if err != nil {
			return CommandResult{}, apperror.InvalidRequest("git executable was not found")
		}
	}
	if r.Timeout <= 0 {
		r.Timeout = gitTimeout
	}
	if r.OutputLimit <= 0 {
		r.OutputLimit = defaultOutputLimit
	}

	runCtx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	remaining := r.OutputLimit
	exceeded := false
	var mu sync.Mutex
	stdout := &limitedBuffer{remaining: &remaining, exceeded: &exceeded, mu: &mu}
	stderr := &limitedBuffer{remaining: &remaining, exceeded: &exceeded, mu: &mu}

	commandArgs := append([]string{"-C", absCWD}, args...)
	cmd := exec.CommandContext(runCtx, gitPath, commandArgs...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err = cmd.Run()
	result := CommandResult{
		Stdout: strings.TrimSpace(stdout.String()),
		Stderr: strings.TrimSpace(stderr.String()),
	}
	if runCtx.Err() != nil {
		return result, apperror.InvalidRequest("git command timed out")
	}
	if exceeded || errors.Is(err, errOutputLimit) {
		return result, apperror.InvalidRequest("git command output exceeded limit")
	}
	if err != nil {
		return result, apperror.InvalidRequest(gitFailureMessage(result.Stderr))
	}
	return result, nil
}

type limitedBuffer struct {
	buf       bytes.Buffer
	remaining *int64
	exceeded  *bool
	mu        *sync.Mutex
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if *b.remaining <= 0 {
		*b.exceeded = true
		return 0, errOutputLimit
	}
	if int64(len(p)) > *b.remaining {
		allowed := int(*b.remaining)
		_, _ = b.buf.Write(p[:allowed])
		*b.remaining = 0
		*b.exceeded = true
		return allowed, errOutputLimit
	}
	written, err := b.buf.Write(p)
	*b.remaining -= int64(written)
	return written, err
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func gitFailureMessage(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return "git command failed"
	}
	const maxMessageLength = 300
	if len(stderr) > maxMessageLength {
		return stderr[:maxMessageLength] + "..."
	}
	return stderr
}
