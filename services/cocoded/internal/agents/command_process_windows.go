//go:build windows

package agents

import (
	"os/exec"
	"time"
)

func configureCommandProcess(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
}
