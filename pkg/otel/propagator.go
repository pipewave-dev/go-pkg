package otel

import (
	"context"

	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type cusMapCarrier struct {
	propagation.MapCarrier
}

func (c cusMapCarrier) Encode() []byte {
	if len(c.MapCarrier) == 0 {
		return nil
	}
	d, e := msgpack.Marshal(c.MapCarrier)
	if e != nil {
		panic(e)
	}
	return d
}

func (c *cusMapCarrier) Decode(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	c.MapCarrier = make(propagation.MapCarrier)
	return msgpack.Unmarshal(data, &c.MapCarrier)
}

func (s *otelClient) Propagation(ctx context.Context) []byte {
	carrier := cusMapCarrier{
		make(propagation.MapCarrier),
	}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier.Encode()
}

// Extract extracts the context from the carrier.
func (s *otelClient) Extract(ctx context.Context, carrier []byte) (context.Context, error) {
	if len(carrier) == 0 {
		return ctx, nil
	}
	propCr := &cusMapCarrier{}
	if err := propCr.Decode(carrier); err != nil {
		return nil, err
	}
	ctx = otel.GetTextMapPropagator().Extract(ctx, propCr)
	return ctx, nil
}
