// +build !windows

package client

import (
    "os/exec"
    "syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Setpgid: true,
    }
}
