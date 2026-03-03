package actx

import "github.com/pipewave-dev/go-pkg/shared/utils/fn"

func (a *aContext) SetTraceID(traceId string) {
	a.data.traceId = traceId
}

func (a *aContext) RefreshTraceId() {
	if a.data.parentTraceId == nil {
		a.data.parentTraceId = []string{}
	}
	a.data.parentTraceId = append(a.data.parentTraceId, a.data.traceId)

	newTraceId := fn.NewNanoID()
	a.data.traceId = newTraceId
}

func (a *aContext) GetTraceID() string {
	return a.data.traceId
}

func (a *aContext) GetParentTraceID() []string {
	return a.data.parentTraceId
}
