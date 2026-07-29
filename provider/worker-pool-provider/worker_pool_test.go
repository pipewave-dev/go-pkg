package workerpoolprovider

import (
	"runtime"
	"testing"
)

func TestResolveWorkers_ExplicitConfigWins(t *testing.T) {
	for _, want := range []int{1, 3, 512} {
		if got := resolveWorkers(want); got != want {
			t.Errorf("resolveWorkers(%d) = %d, want %d", want, got, want)
		}
	}
}

// Unset (0) must derive from GOMAXPROCS — which respects the cgroup CPU quota —
// never from runtime.NumCPU(), which reports host cores and would oversubscribe
// a CPU-limited pod.
func TestResolveWorkers_DerivesFromGOMAXPROCS(t *testing.T) {
	orig := runtime.GOMAXPROCS(0)
	t.Cleanup(func() { runtime.GOMAXPROCS(orig) })

	runtime.GOMAXPROCS(minWorkers * 4)

	got := resolveWorkers(0)
	if want := minWorkers * 4; got != want {
		t.Errorf("resolveWorkers(0) = %d, want %d (GOMAXPROCS)", got, want)
	}
}

// A tight CPU limit drives GOMAXPROCS to 1. Sizing the pool at 1 would let a
// single blocked socket read stall every other connection, so a floor applies.
func TestResolveWorkers_FloorsAtMinWorkers(t *testing.T) {
	orig := runtime.GOMAXPROCS(0)
	t.Cleanup(func() { runtime.GOMAXPROCS(orig) })

	runtime.GOMAXPROCS(1)

	if got := resolveWorkers(0); got != minWorkers {
		t.Errorf("resolveWorkers(0) with GOMAXPROCS=1 = %d, want floor %d", got, minWorkers)
	}
}
