package moduledelivery

import configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"

func (m *moduleDelivery) SetFns(fns *configprovider.Fns) {
	m.c.SetFns(fns)
}
