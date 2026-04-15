package mediatorsvc

import "time"

const pingSweepInterval = 10 * time.Second

func (m *mediatorSvc) startPingLoop() func() {
	ticker := time.NewTicker(pingSweepInterval)
	done := make(chan struct{})

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.wpool.Submit(func() {
					m.PingAllLocalConnections()
				})
			case <-done:
				return
			}
		}
	}()

	return func() {
		close(done)
	}
}
