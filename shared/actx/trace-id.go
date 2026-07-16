package actx

import "github.com/pipewave-dev/go-pkg/shared/utils/fn"

func (a *aContext) SetTraceID(traceId string) {
	a.data.m.Lock()
	defer a.data.m.Unlock()
	a.data.traceId = traceId
}

func (a *aContext) RefreshTraceId() {
	a.data.m.Lock()
	defer a.data.m.Unlock()
	if a.data.parentTraceId == nil {
		a.data.parentTraceId = []string{}
	}
	a.data.parentTraceId = append(a.data.parentTraceId, a.data.traceId)

	newTraceId := fn.NewNanoID()
	a.data.traceId = newTraceId
}

func (a *aContext) GetTraceID() string {
	a.data.m.Lock()
	defer a.data.m.Unlock()
	return a.data.traceId
}

func (a *aContext) GetParentTraceID() []string {
	a.data.m.Lock()
	defer a.data.m.Unlock()
	return a.data.parentTraceId
}
