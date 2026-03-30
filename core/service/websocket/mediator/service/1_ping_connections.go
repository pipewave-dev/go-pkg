package mediatorsvc

func (m *mediatorSvc) PingConnections() {
	// TODO: broadcast to ping all
	conns := m.connections.GetAllConnections()
	for _, conn := range conns {
		if conn == nil {
			continue
		}
		conn.Ping()
	}
}
