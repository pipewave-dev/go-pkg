package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	pipewave "github.com/pipewave-dev/go-pkg"
	"github.com/pipewave-dev/go-pkg/export/adapters"
	pubsubvalkey "github.com/pipewave-dev/go-pkg/export/adapters/pubsub/valkey"
	queuevalkey "github.com/pipewave-dev/go-pkg/export/adapters/queue/valkey"
	dynamorepo "github.com/pipewave-dev/go-pkg/export/adapters/repo/dynamodb"
	pgrepo "github.com/pipewave-dev/go-pkg/export/adapters/repo/postgresql"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/pipewave-dev/go-pkg/server/authn"
	serverconfig "github.com/pipewave-dev/go-pkg/server/config"
	serverfns "github.com/pipewave-dev/go-pkg/server/fns"
	"github.com/pipewave-dev/go-pkg/server/restapi"
	"github.com/pipewave-dev/go-pkg/server/webhook"
)

func main() {
	configFlag := flag.String("config", "config.yaml", "comma-separated list of YAML config files (later override earlier)")
	flag.Parse()
	files := strings.Split(*configFlag, ",")

	srvCfg, err := serverconfig.Load(files)
	if err != nil {
		fatal("load server config", err)
	}

	pw := pipewave.New(pipewave.PipewaveConfig{
		ConfigStore:       pipewave.ConfigFromYAML(files),
		RepositoryFactory: repoAdapter(srvCfg.Repository),
		QueueFactory:      queuevalkey.QueueValkey,
		PubsubFactory:     pubsubvalkey.PubsubValkey,
	})

	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	var signer *webhook.Signer
	if srvCfg.Callbacks.Signature.Mode == serverconfig.SignatureModeEnabled {
		signer, err = webhook.LoadOrGenerateSigner(srvCfg.Callbacks.Signature.SigningKeyFile)
		if err != nil {
			fatal("init webhook signer", err)
		}
	}
	sender := webhook.NewSender(srvCfg.Callbacks.BaseURL, signer)
	if obs, ok := pw.CallbackObserver().(webhook.CallObserver); ok {
		sender.SetObserver(obs)
	}

	asyncBackoff := webhook.DefaultBackoff
	if len(srvCfg.Callbacks.AsyncBackoff) > 0 {
		asyncBackoff = srvCfg.Callbacks.AsyncBackoff
	}
	async := webhook.NewAsyncDispatcher(sender, srvCfg.Callbacks.AsyncRetryMax, asyncBackoff)

	breaker := webhook.NewCircuitBreaker(srvCfg.Callbacks.Breaker.Threshold, srvCfg.Callbacks.Breaker.Cooldown)
	if cm, ok := pw.CallbackObserver().(*metrics.CallbackMetrics); ok {
		if gaugeErr := cm.RegisterBreakerGauge(breaker); gaugeErr != nil {
			slog.Warn("metrics: register breaker gauge failed", "error", gaugeErr)
		}
	}
	syncCaller := webhook.NewSyncCaller(sender, breaker,
		srvCfg.Callbacks.SyncRetry.Max, srvCfg.Callbacks.SyncRetry.Backoff)

	var unhealthyDueToBackend atomic.Bool
	onUnhealthy := func() {
		slog.Error("[pipewave-server] backend unhealthy (log-only)")
	}
	if srvCfg.Callbacks.UnhealthyAction == serverconfig.UnhealthyActionShutdown {
		onUnhealthy = func() {
			slog.Error("[pipewave-server] backend unhealthy — initiating shutdown")
			unhealthyDueToBackend.Store(true)
			stopSignals() // cancel rootCtx → reuse the graceful shutdown path
		}
	}
	monitor := webhook.NewHealthMonitor(onUnhealthy)

	var pinger *webhook.Pinger
	if srvCfg.Callbacks.Ping.Enabled {
		pingURL := srvCfg.Callbacks.BaseURL + srvCfg.Callbacks.Ping.Path
		pingSender := webhook.NewSender(pingURL, signer)
		if obs, ok := pw.CallbackObserver().(webhook.CallObserver); ok {
			pingSender.SetObserver(obs)
		}
		pinger = webhook.NewPinger(pingSender, srvCfg.Callbacks.Ping.Timeout, srvCfg.Callbacks.Ping.FailThreshold)
		if srvCfg.Callbacks.Ping.BootCheck {
			bootCtx, cancel := context.WithTimeout(rootCtx, srvCfg.Callbacks.Ping.Timeout)
			err := pinger.Ping(bootCtx)
			cancel()
			if err != nil {
				fatal("callback ping", err)
			}
			slog.Info("[pipewave-server] callback ping OK")
		}
	}

	fnsCfg := serverfns.Config{
		HandleMessageMode:    srvCfg.Callbacks.HandleMessage.Mode,
		HandleMessageTimeout: srvCfg.Callbacks.HandleMessage.Timeout,
		SyncTimeout:          srvCfg.Callbacks.SyncTimeout,
	}

	if srvCfg.Auth.Mode == serverconfig.AuthModeJWT {
		inspector, err := authn.NewJWTInspector(rootCtx, authn.JWTConfig{
			JWKSURL:          srvCfg.Auth.JWT.JWKSURL,
			PublicKeyPEMFile: srvCfg.Auth.JWT.PublicKeyPEMFile,
			UserIDClaim:      srvCfg.Auth.JWT.UserIDClaim,
			MetadataClaims:   srvCfg.Auth.JWT.MetadataClaims,
		})
		if err != nil {
			fatal("init jwt inspector", err)
		}
		fnsCfg.InspectTokenOverride = inspector.InspectToken
	}

	pw.SetFns(serverfns.New(syncCaller, async, fnsCfg))

	if err := pw.RunMigration(); err != nil {
		fatal("run migration", err)
	}

	clientSrv := &http.Server{Addr: srvCfg.ClientAddr, Handler: pw.Mux()}
	muxCfg := restapi.MuxConfig{APIKeys: srvCfg.APIKeys}
	if signer != nil {
		muxCfg.PublicKey = signer.PublicKey()
	}
	muxCfg.ExtraHealthy = monitor.IsHealthy
	adminSrv := &http.Server{Addr: srvCfg.AdminAddr, Handler: restapi.NewAdminMux(pw, muxCfg)}

	go serve("client", clientSrv)
	go serve("admin", adminSrv)

	go func() {
		// Log, never fatal: a metrics listener that cannot bind must not take
		// the server down.
		if metricsErr := pw.ServeMetrics(); metricsErr != nil {
			slog.Error("[pipewave-server] metrics listener stopped", "error", metricsErr)
		}
	}()

	if pinger != nil {
		go pinger.Run(rootCtx, srvCfg.Callbacks.Ping.Interval, monitor.SetHealthy,
			func() { monitor.SetUnhealthy("ping failed >= threshold") })
	}
	if srvCfg.Callbacks.BreakerOpenShutdown > 0 {
		go webhook.WatchBreakerOpen(rootCtx, breaker, srvCfg.Callbacks.BreakerOpenShutdown, monitor)
	}

	<-rootCtx.Done()
	slog.Info("[pipewave-server] shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = adminSrv.Shutdown(shutdownCtx)
	_ = clientSrv.Shutdown(shutdownCtx)
	_ = pw.ShutdownMetrics(shutdownCtx)
	pw.Shutdown()
	async.Shutdown(shutdownCtx)
	slog.Info("[pipewave-server] bye")

	if unhealthyDueToBackend.Load() {
		os.Exit(1)
	}
}

func repoAdapter(name string) adapters.RepositoryAdapter {
	if name == serverconfig.RepositoryDynamoDB {
		return dynamorepo.DynamoRepo
	}
	return pgrepo.PostgresRepo
}

func serve(name string, srv *http.Server) {
	slog.Info("[pipewave-server] listening", "listener", name, "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal(name+" listener", err)
	}
}

func fatal(what string, err error) {
	slog.Error("[pipewave-server] fatal: "+what, "error", err)
	os.Exit(1)
}
