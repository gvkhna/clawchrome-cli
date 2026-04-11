//go:build !windows

package client

import "syscall"

func bridgeSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
