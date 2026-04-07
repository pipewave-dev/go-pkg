package mediatorsvc

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	"github.com/pipewave-dev/go-pkg/core/repository"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func TestFindUserConnRunsLocalActionForLocalTempDisconnectedSession(t *testing.T) {
	t.Parallel()

	localCalled := 0
	var remoteContainerIDs []string

	helper := &findUserConn{
		ctx:    context.Background(),
		userID: "user-a",
		localAction: func() {
			localCalled++
		},
		targetContainerAction: func(containerIDs []string) {
			remoteContainerIDs = append(remoteContainerIDs, containerIDs...)
		},
		c:           testConfigStore("container-local"),
		connections: fakeConnectionManager{},
		activeConnRepo: fakeActiveConnStore{
			activeByUser: map[string][]entities.ActiveConnection{
				"user-a": {
					{
						UserID:     "user-a",
						InstanceID: "session-1",
						HolderID:   "container-local",
						Status:     voWs.WsStatusTempDisconnected,
					},
				},
			},
		},
	}

	if err := helper.findThenAction(); err != nil {
		t.Fatalf("findThenAction returned error: %v", err)
	}

	if localCalled != 1 {
		t.Fatalf("expected localAction to run once, got %d", localCalled)
	}
	if len(remoteContainerIDs) != 0 {
		t.Fatalf("expected no remote containers, got %v", remoteContainerIDs)
	}
}

func TestFindUserConnKeepsRemoteRoutingForRemoteSessions(t *testing.T) {
	t.Parallel()

	localCalled := 0
	var remoteContainerIDs []string

	helper := &findUserConn{
		ctx:    context.Background(),
		userID: "user-a",
		localAction: func() {
			localCalled++
		},
		targetContainerAction: func(containerIDs []string) {
			remoteContainerIDs = append(remoteContainerIDs, containerIDs...)
		},
		c:           testConfigStore("container-local"),
		connections: fakeConnectionManager{},
		activeConnRepo: fakeActiveConnStore{
			activeByUser: map[string][]entities.ActiveConnection{
				"user-a": {
					{
						UserID:     "user-a",
						InstanceID: "session-1",
						HolderID:   "container-remote",
						Status:     voWs.WsStatusConnected,
					},
					{
						UserID:     "user-a",
						InstanceID: "session-2",
						HolderID:   "container-remote",
						Status:     voWs.WsStatusTempDisconnected,
					},
				},
			},
		},
	}

	if err := helper.findThenAction(); err != nil {
		t.Fatalf("findThenAction returned error: %v", err)
	}

	if localCalled != 0 {
		t.Fatalf("expected localAction to stay untouched, got %d calls", localCalled)
	}
	if !slices.Equal(remoteContainerIDs, []string{"container-remote"}) {
		t.Fatalf("unexpected remote containers: %v", remoteContainerIDs)
	}
}

func testConfigStore(containerID string) configprovider.ConfigStore {
	return configprovider.FromGoStruct(configprovider.EnvType{
		ContainerID: containerID,
		ActiveConnection: configprovider.ActiveConnectionT{
			HeartbeatCutoff: time.Minute,
			PendingMsgTTL:   2 * time.Minute,
		},
		RateLimiter: configprovider.RateLimiterT{
			UserRate:       1,
			UserBurst:      1,
			AnonymousRate:  1,
			AnonymousBurst: 1,
		},
	})
}

type fakeActiveConnStore struct {
	activeByUser map[string][]entities.ActiveConnection
}

func (f fakeActiveConnStore) CountActiveConnections(context.Context, string) (int, aerror.AError) {
	panic("unexpected CountActiveConnections call")
}

func (f fakeActiveConnStore) CountTotalActiveConnections(context.Context) (int64, aerror.AError) {
	panic("unexpected CountTotalActiveConnections call")
}

func (f fakeActiveConnStore) AddConnection(context.Context, string, string, voWs.WsCoreType) aerror.AError {
	panic("unexpected AddConnection call")
}

func (f fakeActiveConnStore) RemoveConnection(context.Context, string, string) aerror.AError {
	panic("unexpected RemoveConnection call")
}

func (f fakeActiveConnStore) UpdateHeartBeat(context.Context, string, string) aerror.AError {
	panic("unexpected UpdateHeartBeat call")
}

func (f fakeActiveConnStore) UpdateStatus(context.Context, string, string, voWs.WsStatus) aerror.AError {
	panic("unexpected UpdateStatus call")
}

func (f fakeActiveConnStore) UpdateStatusTransferring(context.Context, string, string) aerror.AError {
	panic("unexpected UpdateStatusTransferring call")
}

func (f fakeActiveConnStore) CountActiveConnectionsBatch(context.Context, []string) (map[string]int, aerror.AError) {
	panic("unexpected CountActiveConnectionsBatch call")
}

func (f fakeActiveConnStore) GetActiveConnections(_ context.Context, userID string) ([]entities.ActiveConnection, aerror.AError) {
	return f.activeByUser[userID], nil
}

func (f fakeActiveConnStore) GetActiveConnectionsByUserIDs(context.Context, []string) ([]entities.ActiveConnection, aerror.AError) {
	panic("unexpected GetActiveConnectionsByUserIDs call")
}

func (f fakeActiveConnStore) GetInstanceConnection(context.Context, string, string) (*entities.ActiveConnection, aerror.AError) {
	panic("unexpected GetInstanceConnection call")
}

func (f fakeActiveConnStore) CleanUpExpiredConnections(context.Context) aerror.AError {
	panic("unexpected CleanUpExpiredConnections call")
}

var _ repository.ActiveConnStore = fakeActiveConnStore{}

type fakeConnectionManager struct{}

func (fakeConnectionManager) AddConnection(wsSv.WebsocketConn) {}

func (fakeConnectionManager) RemoveConnection(voAuth.WebsocketAuth) {}

func (fakeConnectionManager) GetConnection(voAuth.WebsocketAuth) (wsSv.WebsocketConn, bool) {
	return nil, false
}

func (fakeConnectionManager) GetAllUserConn(string) []wsSv.WebsocketConn {
	return nil
}

func (fakeConnectionManager) GetAllAnonymousConn() []wsSv.WebsocketConn {
	return nil
}

func (fakeConnectionManager) GetAllAuthenticatedConn() []wsSv.WebsocketConn {
	return nil
}

func (fakeConnectionManager) GetAllConnections() []wsSv.WebsocketConn {
	return nil
}
