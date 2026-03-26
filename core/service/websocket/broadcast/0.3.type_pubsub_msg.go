package broadcast

import (
	"context"

	"github.com/pipewave-dev/go-pkg/shared/actx"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/vmihailenco/msgpack/v5"
)

type msgType string

type pubsubMessage struct {
	targetContainers  []string
	sendAllContainers bool

	context     context.Context
	msgType     msgType
	payload     []byte
	otelCarrier []byte

	di *broadcastDI
}

func channelName(containerID string) channelType {
	return channelType(channelPrefix + containerID)
}

func (p *pubsubMessage) Publish() aerror.AError {
	for _, c := range p.targetContainers {
		if c == p.di.c.Env().ContainerID {
			// Skip publish to self container since we can directly call the handler without going through pubsub.
			continue
		}
		channel := channelName(c)
		err := p.di.pubsub.Publish(p.context, channel, p)
		if err != nil {
			return aerror.New(p.context, aerror.ErrUnexpectedPubsub, err)
		}
	}
	return nil
}

func (p *pubsubMessage) GetMsgType() msgType {
	return p.msgType
}

func (p *pubsubMessage) GetPayload() []byte {
	return p.payload
}

func (p *pubsubMessage) GetContext() context.Context {
	return p.context
}

func (p *pubsubMessage) GetOtelCarrier() []byte {
	return p.otelCarrier
}

func (p pubsubMessage) MarshalMsgpack() ([]byte, error) {
	aCtx := actx.From(p.context)
	return msgpack.Marshal(MsgPackTmp{
		TraceID:     aCtx.GetTraceID(),
		OtelCarrier: p.di.otel.Propagation(p.context),
		Channel:     string(p.msgType),
		Payload:     p.payload,
	})
}

func (p *pubsubMessage) UnmarshalMsgpack(b []byte) error {
	dataT := MsgPackTmp{}
	err := msgpack.Unmarshal(b, &dataT)
	if err != nil {
		return err
	}
	p.msgType = msgType(dataT.Channel)
	p.payload = dataT.Payload
	p.otelCarrier = dataT.OtelCarrier

	ctx := context.Background()
	aCtx := actx.From(ctx)
	aCtx.SetTraceID(dataT.TraceID)
	aCtx.RefreshTraceId()

	p.context = ctx

	return nil
}

func (p *pubsubMessage) Encode() []byte {
	d, e := msgpack.Marshal(p)
	if e != nil {
		panic(e)
	}
	return d
}

func (p *pubsubMessage) Decode(data []byte) error {
	return msgpack.Unmarshal(data, p)
}

// Short field names to reduce payload size.
type MsgPackTmp struct {
	TraceID     string `msgpack:"t"`
	OtelCarrier []byte `msgpack:"o"`
	Channel     string `msgpack:"c"`
	Payload     []byte `msgpack:"p"`
}
