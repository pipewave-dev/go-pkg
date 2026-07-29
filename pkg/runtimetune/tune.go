// Package runtimetune applies process-level tuning that the Go runtime does
// not derive correctly on its own inside a container, and reports what it
// settled on so the values are visible in production logs.
//
// Two things matter for a WebSocket server holding many concurrent
// connections:
//
//   - GOMAXPROCS. The runtime defaults to the host's CPU count, which ignores
//     the cgroup CPU quota. On a 64-core node with a 500m limit that means 64
//     OS threads competing for half a core, producing heavy context switching
//     and CFS throttling.
//   - RLIMIT_NOFILE. Every connection costs at least one file descriptor, on
//     top of the listener, the epoll instance, and every database, cache and
//     HTTP client pool. Exhausting it degrades the whole process, so the soft
//     limit is raised to the hard limit and logged.
package runtimetune

import (
	"log/slog"
	"runtime"

	// Sets GOMAXPROCS from the cgroup CPU quota in its init(), before Apply
	// reads the effective value back.
	_ "go.uber.org/automaxprocs"
)

// Result records what tuning settled on, for logging and tests.
type Result struct {
	GOMAXPROCS int
	NumCPU     int
	// FDSoft/FDHard are the RLIMIT_NOFILE values after tuning. Both are 0 when
	// the limit could not be read (non-unix platforms).
	FDSoft uint64
	FDHard uint64
}

// Apply tunes GOMAXPROCS from the cgroup CPU quota and raises the soft file
// descriptor limit to the hard limit.
//
// It never fails the process: tuning is a best-effort optimisation, and a
// server that cannot read its own cgroup should still start. Every fallback is
// logged at warn so a misconfigured deployment is visible rather than silent.
func Apply() Result {
	res := Result{NumCPU: runtime.NumCPU()}

	// automaxprocs sets GOMAXPROCS from the cgroup CPU quota via its own
	// init(); read back the effective value rather than assuming it applied,
	// since it is a no-op when no quota is set (bare metal, or a pod with no
	// CPU limit) and when GOMAXPROCS is already set in the environment.
	res.GOMAXPROCS = runtime.GOMAXPROCS(0)

	soft, hard, err := getFDLimit()
	if err != nil {
		slog.Warn("runtimetune: cannot read RLIMIT_NOFILE, fd limit left untouched",
			slog.Any("error", err))
		return res
	}

	if soft < hard {
		if newSoft, setErr := raiseSoftLimit(soft, hard); setErr != nil {
			// Common in restricted sandboxes; the process still runs, just
			// with a lower connection ceiling than the host would allow.
			slog.Warn("runtimetune: cannot raise RLIMIT_NOFILE soft limit",
				slog.Uint64("soft", soft),
				slog.Uint64("hard", hard),
				slog.Any("error", setErr))
		} else {
			soft = newSoft
		}
	}

	res.FDSoft, res.FDHard = soft, hard
	return res
}

// practicalFDCeiling bounds the fallback raise. Well above any realistic
// per-pod connection count, but low enough to be accepted by kernels that
// reject an unbounded request.
const practicalFDCeiling = 1 << 20 // 1048576

// raiseSoftLimit lifts the soft limit as close to hard as the kernel allows,
// returning the value actually in effect.
//
// Asking for exactly `hard` is the right first try, but it is rejected in two
// real cases: macOS reports RLIM_INFINITY as the hard limit while capping the
// usable value far lower, and some hardened kernels refuse an infinite
// request. Falling back to a large finite value means those platforms still
// get a workable ceiling instead of being left at the low default, which is
// the whole point of doing this at startup.
func raiseSoftLimit(soft, hard uint64) (uint64, error) {
	err := setFDSoftLimit(hard, hard)
	if err == nil {
		return hard, nil
	}

	want := min(hard, practicalFDCeiling)
	if want <= soft {
		// Nothing better than the current value is on offer; report the
		// original failure rather than pretending this succeeded.
		return soft, err
	}
	if fallbackErr := setFDSoftLimit(want, hard); fallbackErr != nil {
		return soft, fallbackErr
	}
	return want, nil
}

// Log reports the tuned values. Worth doing unconditionally at startup: when a
// production incident turns out to be fd exhaustion or CPU throttling, these
// two lines are what identify it.
func (r Result) Log() {
	slog.Info("runtimetune: applied",
		slog.Int("gomaxprocs", r.GOMAXPROCS),
		slog.Int("num_cpu", r.NumCPU),
		slog.Uint64("fd_soft_limit", r.FDSoft),
		slog.Uint64("fd_hard_limit", r.FDHard))

	if r.GOMAXPROCS == r.NumCPU && r.NumCPU > 2 {
		slog.Warn("runtimetune: GOMAXPROCS equals host CPU count — if this pod has a CPU limit, "+
			"the cgroup quota was not detected and the scheduler will oversubscribe it",
			slog.Int("gomaxprocs", r.GOMAXPROCS))
	}
}
