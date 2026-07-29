//go:build unix

package runtimetune

import "syscall"

// getFDLimit reports the current soft and hard RLIMIT_NOFILE.
func getFDLimit() (soft, hard uint64, err error) {
	var rl syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rl); err != nil {
		return 0, 0, err
	}
	return uint64(rl.Cur), uint64(rl.Max), nil
}

// setFDSoftLimit raises the soft RLIMIT_NOFILE to want, which must not exceed
// the hard limit (an unprivileged process cannot raise the hard limit).
func setFDSoftLimit(want, hard uint64) error {
	rl := syscall.Rlimit{Cur: want, Max: hard}
	return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rl)
}
