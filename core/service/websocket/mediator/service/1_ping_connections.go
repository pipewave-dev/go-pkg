package mediatorsvc

func (m *mediatorSvc) PingAllLocalConnections() {
	conns := m.connections.GetAllConnections()
	for _, conn := range conns {
		if conn == nil {
			continue
		}
		conn.Ping()
	}
}
