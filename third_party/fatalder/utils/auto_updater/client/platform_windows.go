// +build windows

package client

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {
    // Windows 不需要设置 Setpgid，空实现
}
