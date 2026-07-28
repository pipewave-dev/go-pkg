package clientmsghandler

import (
	"testing"

	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/stretchr/testify/require"
)

// outcomeRecorder is the seam the handler writes through.
func TestOutcomeConstantsAreDistinct(t *testing.T) {
	all := []string{
		metrics.OutcomeOK,
		metrics.OutcomeError,
		metrics.OutcomeInvalidSchema,
		metrics.OutcomeDedup,
		metrics.OutcomeRateLimited,
	}
	seen := make(map[string]struct{}, len(all))
	for _, o := range all {
		require.NotEmpty(t, o)
		_, dup := seen[o]
		require.False(t, dup, "duplicate outcome value %q", o)
		seen[o] = struct{}{}
	}
}

// TestDeferOrdering_MetricsRunsAfterSend pins the LIFO defer semantics that
// handleMessage relies on: the metrics defer is registered FIRST and the
// send defer is registered SECOND, so on function return they run in the
// reverse order — send, then metrics — meaning the recorded duration covers
// the write. This does not exercise the real handler (which needs six
// collaborators to construct); it proves the general Go property the
// handler's structure depends on, using local closures that mirror its
// defer registration order exactly.
func TestDeferOrdering_MetricsRunsAfterSend(t *testing.T) {
	var events []string

	func() {
		// Registered first -> runs last, matching handleMessage's metrics defer.
		defer func() {
			events = append(events, "metrics")
		}()

		// Registered second -> runs first, matching handleMessage's send defer.
		defer func() {
			events = append(events, "send")
		}()
	}()

	require.Equal(t, []string{"send", "metrics"}, events)
}
