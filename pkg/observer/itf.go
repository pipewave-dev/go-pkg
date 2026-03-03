package observer

import (
	"context"

	"github.com/pipewave-dev/go-pkg/pkg/observer/fieldvalue"
	"go.opentelemetry.io/otel/trace"
)

type Observability interface {
	StartOperation(
		inCtx context.Context,
		name string,
		opts ...OperationOption,
	) (outCtx context.Context, opbOp Operation)
}

type Operation interface {
	AddFields(field fieldvalue.Field, other ...fieldvalue.Field)
	AddStep(msg string, fields ...fieldvalue.Field)
	Propagation() (carrier []byte)
	Success()
	Failure(err error)
	Finish(err error)
}

// ===

type OperationOption interface {
	Apply(*operationConfig)
}

func DefaultConfig() *operationConfig {
	return &operationConfig{
		fields:   []fieldvalue.Field{},
		spanKind: trace.SpanKindServer,
		carrier:  nil,
	}
}

func WithOpFields(fields ...fieldvalue.Field) OperationOption {
	return opFields(fields)
}
func WithOpKind(kind SpanKind) OperationOption {
	return opKind(kind)
}
func WithCarrier(carrier []byte) OperationOption {
	return opCarrier(carrier)
}

type operationConfig struct {
	fields   []fieldvalue.Field
	spanKind SpanKind
	carrier  []byte
}

func (c *operationConfig) Fields() []fieldvalue.Field {
	return c.fields
}
func (c *operationConfig) SpanKind() SpanKind {
	return c.spanKind
}
func (c *operationConfig) Carrier() []byte {
	return c.carrier
}

type SpanKind = trace.SpanKind

// opFields implements OperationOption for setting fields
type opFields []fieldvalue.Field

func (o opFields) Apply(c *operationConfig) {
	c.fields = append(c.fields, o...)
}

// opKind implements OperationOption for setting span kind
type opKind SpanKind

func (o opKind) Apply(c *operationConfig) {
	c.spanKind = SpanKind(o)
}

// opKind implements OperationOption for setting span kind
type opCarrier []byte

func (o opCarrier) Apply(c *operationConfig) {
	c.carrier = o
}
