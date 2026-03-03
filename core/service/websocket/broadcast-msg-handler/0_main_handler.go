package broadcastmsghandler

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	repo "github.com/pipewave-dev/go-pkg/core/repository"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
)

type broadcastMsgHandler struct {
	storeActiveWs repo.ActiveConnStore
	connections   wsSv.ConnectionManager
}

func New(
	repo repository.AllRepository,
	connections wsSv.ConnectionManager,
) broadcast.PubsubHandler {
	return &broadcastMsgHandler{
		storeActiveWs: repo.ActiveConnStore(),
		connections:   connections,
	}
}
