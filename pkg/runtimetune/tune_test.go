package runtimetune

import (
	"runtime"
	"testing"
)

func TestApply_ReportsRuntimeValues(t *testing.T) {
	res := Apply()

	if res.NumCPU != runtime.NumCPU() {
		t.Errorf("NumCPU = %d, want %d", res.NumCPU, runtime.NumCPU())
	}
	if res.GOMAXPROCS < 1 {
		t.Errorf("GOMAXPROCS = %d, want >= 1", res.GOMAXPROCS)
	}
	if res.GOMAXPROCS != runtime.GOMAXPROCS(0) {
		t.Errorf("GOMAXPROCS = %d, does not match runtime value %d",
			res.GOMAXPROCS, runtime.GOMAXPROCS(0))
	}
}

// On unix, Apply must leave the soft limit equal to the hard limit (or explain
// itself via a warn log). A soft limit still below hard means the raise
// silently failed to take effect.
func TestApply_RaisesFDSoftLimit(t *testing.T) {
	if _, _, err := getFDLimit(); err != nil {
		t.Skipf("RLIMIT_NOFILE unavailable: %v", err)
	}

	res := Apply()

	if res.FDHard == 0 {
		t.Fatal("FDHard = 0, want the platform hard limit")
	}
	if res.FDSoft > res.FDHard {
		t.Errorf("FDSoft = %d exceeds FDHard = %d", res.FDSoft, res.FDHard)
	}

	// Confirm Apply actually mutated process state, not just its return value.
	soft, hard, err := getFDLimit()
	if err != nil {
		t.Fatalf("getFDLimit after Apply: %v", err)
	}
	if soft != res.FDSoft || hard != res.FDHard {
		t.Errorf("Apply reported soft=%d hard=%d, process has soft=%d hard=%d",
			res.FDSoft, res.FDHard, soft, hard)
	}
}

// raiseSoftLimit must leave the process no worse off than it started, on every
// platform: either the soft limit went up, or it stayed put and an error
// explains why. Silently reporting a raise that did not happen would make the
// startup log lie about the connection ceiling.
func TestRaiseSoftLimit_NeverRegresses(t *testing.T) {
	orig, hard, err := getFDLimit()
	if err != nil {
		t.Skipf("RLIMIT_NOFILE unavailable: %v", err)
	}

	// A previous test (or Apply itself) may already have pushed the soft limit
	// to the hard limit, leaving nothing to raise. Lower it deliberately so
	// there is real headroom, and restore it afterwards. 256 stays above the
	// handful of fds the test binary itself needs.
	const lowered = 256
	if lowered >= hard {
		t.Skipf("hard limit %d too low to exercise a raise", hard)
	}
	if err := setFDSoftLimit(lowered, hard); err != nil {
		t.Skipf("cannot lower soft limit to set up the test: %v", err)
	}
	t.Cleanup(func() { _ = setFDSoftLimit(orig, hard) })

	soft := uint64(lowered)
	got, raiseErr := raiseSoftLimit(soft, hard)

	if got < soft {
		t.Errorf("raiseSoftLimit returned %d, below the starting soft limit %d", got, soft)
	}
	if raiseErr != nil && got != soft {
		t.Errorf("raiseSoftLimit failed (%v) but reported a changed limit %d, want the original %d",
			raiseErr, got, soft)
	}

	// Whatever it reported must match actual process state.
	nowSoft, _, err := getFDLimit()
	if err != nil {
		t.Fatalf("getFDLimit: %v", err)
	}
	if nowSoft != got {
		t.Errorf("raiseSoftLimit reported %d, process has %d", got, nowSoft)
	}
}

// Apply runs at startup and may be called again by tests or embedders; it must
// not regress the limit it already raised.
func TestApply_Idempotent(t *testing.T) {
	if _, _, err := getFDLimit(); err != nil {
		t.Skipf("RLIMIT_NOFILE unavailable: %v", err)
	}

	first := Apply()
	second := Apply()

	if first != second {
		t.Errorf("Apply not idempotent: first = %+v, second = %+v", first, second)
	}
}
