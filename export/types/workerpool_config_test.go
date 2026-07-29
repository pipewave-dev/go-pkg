package types

import "testing"

func TestWorkerPoolT_LoadDefault_DerivesThresholdsFromBuffer(t *testing.T) {
	w := &WorkerPoolT{}
	w.loadDefault()

	if w.Buffer <= 64 {
		t.Errorf("default Buffer = %d, want a production-sized queue well above the old 64", w.Buffer)
	}
	if w.UpperThreshold >= w.Buffer {
		t.Errorf("UpperThreshold = %d, want below Buffer %d so backpressure trips before the queue drops tasks",
			w.UpperThreshold, w.Buffer)
	}
	if w.UpperThreshold <= w.LowerThreshold {
		t.Errorf("UpperThreshold = %d must exceed LowerThreshold = %d", w.UpperThreshold, w.LowerThreshold)
	}
	w.validate() // must not panic
}

// An explicit Buffer must scale the derived thresholds with it, not leave them
// at values tuned for a different queue depth.
func TestWorkerPoolT_LoadDefault_ScalesWithExplicitBuffer(t *testing.T) {
	w := &WorkerPoolT{Buffer: 800}
	w.loadDefault()

	if w.UpperThreshold != 600 {
		t.Errorf("UpperThreshold = %d, want 600 (3/4 of 800)", w.UpperThreshold)
	}
	if w.LowerThreshold != 200 {
		t.Errorf("LowerThreshold = %d, want 200 (1/4 of 800)", w.LowerThreshold)
	}
	w.validate()
}

// Explicit values must survive loadDefault untouched.
func TestWorkerPoolT_LoadDefault_KeepsExplicitThresholds(t *testing.T) {
	w := &WorkerPoolT{Buffer: 1000, UpperThreshold: 900, LowerThreshold: 100}
	w.loadDefault()

	if w.UpperThreshold != 900 || w.LowerThreshold != 100 {
		t.Errorf("thresholds = (%d, %d), want the configured (900, 100)",
			w.UpperThreshold, w.LowerThreshold)
	}
}

// Tiny buffers are legal (tests, embedded use). The derived ratios collapse at
// that size, so loadDefault must still hand validate a usable pair rather than
// panicking on a config the user never wrote.
func TestWorkerPoolT_LoadDefault_TinyBufferStaysValid(t *testing.T) {
	for _, buf := range []int{1, 2, 3, 4, 8} {
		w := &WorkerPoolT{Buffer: buf}
		w.loadDefault()

		if w.UpperThreshold <= w.LowerThreshold {
			t.Errorf("Buffer=%d: UpperThreshold = %d must exceed LowerThreshold = %d",
				buf, w.UpperThreshold, w.LowerThreshold)
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Buffer=%d: validate panicked: %v", buf, r)
				}
			}()
			w.validate()
		}()
	}
}

func TestWorkerPoolT_Validate_RejectsNegativeWorkers(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("validate accepted negative Workers, want panic")
		}
	}()
	w := &WorkerPoolT{Workers: -1, Buffer: 10, UpperThreshold: 8, LowerThreshold: 2}
	w.validate()
}

// Workers=0 is the documented "derive from GOMAXPROCS" signal, not an error.
func TestWorkerPoolT_Validate_AllowsZeroWorkers(t *testing.T) {
	w := &WorkerPoolT{Workers: 0, Buffer: 10, UpperThreshold: 8, LowerThreshold: 2}
	w.validate()
}

func TestActiveConnectionT_Validate_RejectsNegativeMaxConnections(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("validate accepted negative MaxConnections, want panic")
		}
	}()
	m := &ActiveConnectionT{HeartbeatCutoff: 1, PendingMsgTTL: 2, MaxConnections: -1}
	m.validate()
}
