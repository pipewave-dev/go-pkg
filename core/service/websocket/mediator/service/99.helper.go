package mediatorsvc

import (
	"context"
	"errors"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	repo "github.com/pipewave-dev/go-pkg/core/repository"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

type findUserConn struct {
	ctx    context.Context
	userID string

	localAction           func()
	targetContainerAction func(containerIDs []string)

	c              configprovider.ConfigStore
	connections    wsSv.ConnectionManager
	activeConnRepo repo.ActiveConnStore
}

func (f *findUserConn) findThenAction() aerror.AError {
	actConns, aErr := f.activeConnRepo.GetActiveConnections(f.ctx, f.userID)
	if aErr != nil && !errors.Is(aErr, aerror.RecordNotFound) {
		return aErr
	}
	// Check connection in memory first
	userConns := f.connections.GetAllUserConn(f.userID)
	if len(userConns) > 0 {
		f.localAction()
	}

	containerIDs := make([]string, 0, len(actConns))
	seen := make(map[string]struct{}, len(actConns))
	for _, conn := range actConns {
		if _, ok := seen[conn.HolderID]; ok {
			continue
		}
		seen[conn.HolderID] = struct{}{}
		if conn.HolderID != "" && conn.HolderID != f.c.Env().ContainerID {
			containerIDs = append(containerIDs, conn.HolderID)
		}
	}

	if len(containerIDs) > 0 {
		f.targetContainerAction(containerIDs)
	}
	return nil
}

type findSessionConn struct {
	ctx              context.Context
	userID           string
	instanceID       string
	callbackNotfound func() // Only need when find by instanceID

	localAction           func()
	targetContainerAction func(containerIDs []string)

	c              configprovider.ConfigStore
	connections    wsSv.ConnectionManager
	activeConnRepo repo.ActiveConnStore
}

func (f *findSessionConn) findThenAction() aerror.AError {
	tmpAuth := voAuth.UserWebsocketAuth(f.userID, f.instanceID)
	// Check connection in memory first
	_, ok := f.connections.GetConnection(tmpAuth)
	if ok {
		f.localAction()
		return nil
	}

	// If not found, check in active connection repo
	actConn, aErr := f.activeConnRepo.GetInstanceConnection(f.ctx, f.userID, f.instanceID)
	if aErr != nil {
		if errors.Is(aErr, aerror.RecordNotFound) {
			f.callbackNotfound()
			return nil
		}
		return aErr
	}

	f.targetContainerAction([]string{actConn.HolderID})
	return nil
}

type findMultiUserConn struct {
	ctx     context.Context
	userIDs []string

	localAction           func()
	targetContainerAction func(containerIDs []string)

	c              configprovider.ConfigStore
	connections    wsSv.ConnectionManager
	activeConnRepo repo.ActiveConnStore
}

func (f *findMultiUserConn) findThenAction() aerror.AError {
	actConns, aErr := f.activeConnRepo.GetActiveConnectionsByUserIDs(f.ctx, f.userIDs)
	if aErr != nil && !errors.Is(aErr, aerror.RecordNotFound) {
		return aErr
	}

	// Check connection in memory first
	for _, userID := range f.userIDs {
		userConns := f.connections.GetAllUserConn(userID)
		if len(userConns) > 0 {
			f.localAction()
			break
		}
	}

	containerIDs := make([]string, 0, len(actConns))
	seen := make(map[string]struct{}, len(actConns))
	for _, conn := range actConns {
		if _, ok := seen[conn.HolderID]; ok {
			continue
		}
		seen[conn.HolderID] = struct{}{}
		if conn.HolderID != "" && conn.HolderID != f.c.Env().ContainerID {
			containerIDs = append(containerIDs, conn.HolderID)
		}
	}

	if len(containerIDs) > 0 {
		f.targetContainerAction(containerIDs)
	}
	return nil
}
