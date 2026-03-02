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
)

// New creates a new worker pool provider with injected config and dependencies.
// This replaces the singleton pattern in singleton/worker-pool with dependency injection.
func New(
	cfg configprovider.ConfigStore,
	healthy healthyprovider.Healthy,
	cleanupTask fncollector.CleanupTask,
) *workerpool.WorkerPool {
	env := cfg.Env()

	poolCfg := &workerpool.WorkerPoolCfn{
		Workers: runtime.NumCPU(),
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

	// Shutdown worker pool before other dependencies
	cleanupTask.RegTask(ins.Close, fncollector.FnPriorityEarly)

	return ins
}
