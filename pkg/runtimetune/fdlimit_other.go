//go:build !unix

package runtimetune

import "errors"

var errUnsupported = errors.New("RLIMIT_NOFILE is not supported on this platform")

func getFDLimit() (soft, hard uint64, err error) { return 0, 0, errUnsupported }

func setFDSoftLimit(want, hard uint64) error { return errUnsupported }
