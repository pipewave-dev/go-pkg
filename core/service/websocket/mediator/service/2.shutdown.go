package mediatorsvc

// Shutdown performs graceful shutdown of the mediator service
func (m *mediatorSvc) Shutdown() {
	// 2. Close all existing connections
	allConnections := m.connections.GetAllConnections()
	for _, conn := range allConnections {
		conn.Close()
	}
}
