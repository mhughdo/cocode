package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/apperror"
)

func TestRunnerRunCapturesGitOutput(t *testing.T) {
	t.Parallel()

	repoPath := initGitRepo(t)
	result, err := DefaultRunner().Run(context.Background(), repoPath, "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Stdout != repoPath {
		t.Fatalf("Stdout = %q, want %q", result.Stdout, repoPath)
	}
	if result.Stderr != "" {
		t.Fatalf("Stderr = %q, want empty", result.Stderr)
	}
}

func TestRunnerMapsInvalidCWDToTypedError(t *testing.T) {
	t.Parallel()

	_, err := DefaultRunner().Run(context.Background(), filepath.Join(t.TempDir(), "missing"), "status")
	assertAppError(t, err, apperror.CodeInvalidRequest, "cwd does not exist")
}

func TestRunnerEnforcesOutputLimit(t *testing.T) {
	t.Parallel()

	repoPath := initGitRepo(t)
	runner := DefaultRunner()
	runner.OutputLimit = 4

	_, err := runner.Run(context.Background(), repoPath, "rev-parse", "--show-toplevel")
	assertAppError(t, err, apperror.CodeInvalidRequest, "output exceeded limit")
}

func TestRunnerEnforcesTimeout(t *testing.T) {
	t.Parallel()

	script := writeFakeGit(t, "#!/bin/sh\nsleep 2\n")
	runner := Runner{
		GitPath:     script,
		Timeout:     10 * time.Millisecond,
		OutputLimit: 1024,
	}

	_, err := runner.Run(context.Background(), t.TempDir(), "status")
	assertAppError(t, err, apperror.CodeInvalidRequest, "timed out")
}

func TestRunnerMapsExitErrorToTypedError(t *testing.T) {
	t.Parallel()

	script := writeFakeGit(t, "#!/bin/sh\necho 'fatal: no repo here' >&2\nexit 7\n")
	runner := Runner{
		GitPath:     script,
		Timeout:     time.Second,
		OutputLimit: 1024,
	}

	_, err := runner.Run(context.Background(), t.TempDir(), "status")
	assertAppError(t, err, apperror.CodeInvalidRequest, "fatal: no repo here")
}

func writeFakeGit(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func assertAppError(t *testing.T, err error, code apperror.Code, messageContains string) {
	t.Helper()

	if err == nil {
		t.Fatal("error = nil, want *apperror.Error")
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %T, want *apperror.Error", err)
	}
	if appErr.Code != code {
		t.Fatalf("Code = %q, want %q", appErr.Code, code)
	}
	if !strings.Contains(appErr.Message, messageContains) {
		t.Fatalf("Message = %q, want to contain %q", appErr.Message, messageContains)
	}
}
