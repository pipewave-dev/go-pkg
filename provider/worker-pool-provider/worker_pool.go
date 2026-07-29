package workerpoolprovider

import (
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"

	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	healthyprovider "github.com/pipewave-dev/go-pkg/provider/healthy-provider"
	"github.com/samber/do/v2"
)

// minWorkers floors the pool size. Worker goroutines here are not CPU-bound —
// they block on socket reads and downstream I/O — so sizing them at exactly
// GOMAXPROCS would let a handful of slow reads stall the whole pool and start
// dropping tasks. GOMAXPROCS still bounds actual parallelism, so the extra
// goroutines cost stack, not CPU contention.
const minWorkers = 8

// resolveWorkers picks the worker count: an explicit config value if set,
// otherwise GOMAXPROCS floored at minWorkers.
//
// GOMAXPROCS rather than runtime.NumCPU() is the important part. NumCPU
// reports the host's cores and ignores the cgroup CPU quota, so on a large
// node a pod limited to 500m would previously get one worker per host core,
// oversubscribing its quota badly. runtimetune.Apply aligns GOMAXPROCS with
// the quota, so reading it here inherits that.
func resolveWorkers(configured int) int {
	if configured > 0 {
		return configured
	}
	if n := runtime.GOMAXPROCS(0); n > minWorkers {
		return n
	}
	return minWorkers
}

func NewDI(i do.Injector) (*workerpool.WorkerPool, error) {
	cfg := do.MustInvoke[configprovider.ConfigStore](i)
	healthy := do.MustInvoke[healthyprovider.Healthy](i)
	cleanupTask := do.MustInvoke[fncollector.CleanupTask](i)

	env := cfg.Env()

	poolCfg := &workerpool.WorkerPoolCfn{
		Workers: resolveWorkers(env.WorkerPool.Workers),
		Buffer:  env.WorkerPool.Buffer,
		UpperThreshold: workerpool.Threshold{
			Value: env.WorkerPool.UpperThreshold,
			Action: func() {
				healthy.SetUnhealthy("Worker pool is over upper threshold")
			},
		},
		LowerThreshold: workerpool.Threshold{
			Value: env.WorkerPool.LowerThreshold,
			Action: func() {
				healthy.SetHealthy("Worker pool is below lower threshold")
			},
		},
		PanicHandler: func(recover any) {
			dbTrace := fmt.Sprintf("%s\n", debug.Stack())
			var reportBug string
			switch recoverT := recover.(type) {
			case string:
				reportBug = fmt.Sprintf("<< PANIC >> %s \n %s", recoverT, dbTrace)
			case error:
				reportBug = fmt.Sprintf("<< PANIC >> %s \n %s", recoverT.Error(), dbTrace)
			default:
				reportBug = fmt.Sprintf("<< unexpected PANIC >> %s \n %s", recoverT, dbTrace)
			}
			slog.Error(reportBug)
		},
	}

	ins := workerpool.New(poolCfg)
	ins.Start()

	// Close after connection-closing shutdown steps (mediatorSvc.Shutdown,
	// msgHubSvc.Shutdown — both FnPriorityNormal) so they stop submitting
	// new tasks first; Close() then drains whatever is left in the queue.
	// Submit itself is still safe even if called after Close (see
	// WorkerPool.Submit/Close), this ordering just avoids needlessly
	// dropping in-flight tasks from connections that are still closing.
	cleanupTask.RegTask(ins.Close, fncollector.FnPriorityLate)

	return ins, nil
}
