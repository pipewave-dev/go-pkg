package connectionmanager

import (
	"fmt"
	"sync"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	dostuffs "github.com/pipewave-dev/go-pkg/global/do-stuffs"
)

var (
	once     sync.Once
	instance *connectionMap
)

func Singleton() wsSv.ConnectionManager {
	once.Do(func() {
		instance = &connectionMap{
			userConn:      make(map[string]map[string]wsSv.WebsocketConn),
			anonymousConn: make(map[string]wsSv.WebsocketConn),
		}

		dostuffs.DebugFn.RegTask(instance.printStats)
	})
	return instance
}

type connectionMap struct {
	userConn      map[string]map[string]wsSv.WebsocketConn // userID -> sessionID -> conn
	anonymousConn map[string]wsSv.WebsocketConn            // sessionID -> conn
	mu            sync.RWMutex
}

func (m *connectionMap) AddConnection(connection wsSv.WebsocketConn) {
	auth := connection.Auth()
	m.mu.Lock()
	defer m.mu.Unlock()
	if auth.IsAnonymous() {
		m.anonymousConn[auth.InstanceID] = connection
		return
	} else {
		if _, ok := m.userConn[auth.UserID]; !ok {
			m.userConn[auth.UserID] = make(map[string]wsSv.WebsocketConn)
		}
		m.userConn[auth.UserID][auth.InstanceID] = connection
	}
}

func (m *connectionMap) RemoveConnection(auth voAuth.WebsocketAuth) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if auth.IsAnonymous() {
		delete(m.anonymousConn, auth.InstanceID)
		return
	} else {
		if userClients, ok := m.userConn[auth.UserID]; ok {
			delete(userClients, auth.InstanceID)
			if len(userClients) == 0 {
				delete(m.userConn, auth.UserID)
			}
		}
	}
}

func (m *connectionMap) GetConnection(auth voAuth.WebsocketAuth) (conn wsSv.WebsocketConn, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if auth.IsAnonymous() {
		conn, ok = m.anonymousConn[auth.InstanceID]
		return conn, ok
	} else {
		var userClients map[string]wsSv.WebsocketConn
		if userClients, ok = m.userConn[auth.UserID]; ok {
			conn, ok = userClients[auth.InstanceID]
			return conn, ok
		}
	}
	return nil, false
}

func (m *connectionMap) GetAllUserConn(userID string) []wsSv.WebsocketConn {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if userClients, ok := m.userConn[userID]; ok {
		connections := make([]wsSv.WebsocketConn, 0, len(userClients))
		for _, conn := range userClients {
			connections = append(connections, conn)
		}
		return connections
	}
	return nil
}

func (m *connectionMap) GetAllAnonymousConn() []wsSv.WebsocketConn {
	m.mu.RLock()
	defer m.mu.RUnlock()

	connections := make([]wsSv.WebsocketConn, 0, len(m.anonymousConn))
	for _, conn := range m.anonymousConn {
		connections = append(connections, conn)
	}
	return connections
}

func (m *connectionMap) GetAllAuthenticatedConn() []wsSv.WebsocketConn {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var connections []wsSv.WebsocketConn
	for _, userClients := range m.userConn {
		for _, conn := range userClients {
			connections = append(connections, conn)
		}
	}
	return connections
}

func (m *connectionMap) GetAllConnections() []wsSv.WebsocketConn {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allConnections []wsSv.WebsocketConn

	// Add all user connections
	for _, userClients := range m.userConn {
		for _, conn := range userClients {
			allConnections = append(allConnections, conn)
		}
	}

	// Add all anonymous connections
	for _, conn := range m.anonymousConn {
		allConnections = append(allConnections, conn)
	}

	return allConnections
}

func (m *connectionMap) printStats() {
	fmt.Println("=== ConnectionManager Stats ===")
	fmt.Printf("\tUser: %d\n", len(m.userConn))
	for userID, conns := range m.userConn {
		fmt.Printf("\tUserID: %s, Connections: %d\n", userID, len(conns))
	}
	fmt.Printf("\tTotal Anonymous Connections: %d\n", len(m.anonymousConn))
}
