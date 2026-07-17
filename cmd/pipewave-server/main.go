package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	pipewave "github.com/pipewave-dev/go-pkg"
	"github.com/pipewave-dev/go-pkg/export/adapters"
	dynamorepo "github.com/pipewave-dev/go-pkg/export/adapters/repo/dynamodb"
	pgrepo "github.com/pipewave-dev/go-pkg/export/adapters/repo/postgresql"
	pubsubvalkey "github.com/pipewave-dev/go-pkg/export/adapters/pubsub/valkey"
	queuevalkey "github.com/pipewave-dev/go-pkg/export/adapters/queue/valkey"
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

	signer, err := webhook.LoadOrGenerateSigner(srvCfg.Callbacks.SigningKeyFile)
	if err != nil {
		fatal("init webhook signer", err)
	}
	sender := webhook.NewSender(srvCfg.Callbacks.BaseURL, signer)
	async := webhook.NewAsyncDispatcher(sender, srvCfg.Callbacks.AsyncRetryMax, webhook.DefaultBackoff)
	syncCaller := webhook.NewSyncCaller(sender, webhook.NewCircuitBreaker(5, 10*time.Second))

	fnsCfg := serverfns.Config{
		HandleMessageMode:    srvCfg.Callbacks.HandleMessage.Mode,
		HandleMessageTimeout: srvCfg.Callbacks.HandleMessage.Timeout,
		SyncTimeout:          srvCfg.Callbacks.SyncTimeout,
	}
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

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
	adminSrv := &http.Server{Addr: srvCfg.AdminAddr, Handler: restapi.NewAdminMux(pw, restapi.MuxConfig{
		APIKeys:   srvCfg.APIKeys,
		PublicKey: signer.PublicKey(),
	})}

	go serve("client", clientSrv)
	go serve("admin", adminSrv)

	<-rootCtx.Done()
	slog.Info("[pipewave-server] shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = adminSrv.Shutdown(shutdownCtx)
	_ = clientSrv.Shutdown(shutdownCtx)
	pw.Shutdown()
	async.Shutdown(shutdownCtx)
	slog.Info("[pipewave-server] bye")
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
