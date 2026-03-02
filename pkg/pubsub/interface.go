package pubsub

import (
	"context"
	"log/slog"
	"reflect"
)

type NetworkSerializable interface {
	Encode() []byte
	Decode([]byte) error
}

type PubsubProvider[ChannelT ~string, MsgT NetworkSerializable] interface {
	Publish(ctx context.Context, channel ChannelT, message MsgT) error
	Subscribe(channel ChannelT, handler HandlerFn[MsgT]) (unsubscribe func(), err error)
	Healthcheck() error

	Flush()
}

type pubsubProvider[ChannelT ~string, MsgT NetworkSerializable] struct {
	adapter Adapter
}

func New[ChannelT ~string, MsgT NetworkSerializable](adapter Adapter) PubsubProvider[ChannelT, MsgT] {
	ins := &pubsubProvider[ChannelT, MsgT]{
		adapter,
	}
	return ins
}

func (r *pubsubProvider[ChannelT, MsgT]) Publish(ctx context.Context, channel ChannelT, message MsgT) error {
	return r.adapter.Publish(ctx, string(channel), message.Encode())
}

func (r *pubsubProvider[ChannelT, MsgT]) Subscribe(channel ChannelT, handler func(hdlMsg MsgT)) (unsubscribe func(), err error) {
	return r.adapter.Subscribe(string(channel), func(message []byte) {
		handleConverter(message, handler)
	})
}

func (r *pubsubProvider[ChannelT, MsgT]) Flush() {
	r.adapter.Flush()
}
func (r *pubsubProvider[ChannelT, MsgT]) Healthcheck() error {
	return r.adapter.Healthcheck()
}

type HandlerFn[T NetworkSerializable] = func(hdlMsg T)

func handleConverter[T NetworkSerializable](
	message []byte,
	handleFn HandlerFn[T],
) {
	var msg T
	if reflect.TypeOf(msg).Kind() == reflect.Ptr {
		msg = reflect.New(reflect.TypeOf(msg).Elem()).Interface().(T)
	} else {
		msg = reflect.Zero(reflect.TypeOf(msg)).Interface().(T)
	}
	err := msg.Decode(message)
	if err != nil {
		slog.Error("Pubsub Provider.Subscribe NetworkSerializable.Decode", slog.Any("err", err))
		return
	}
	handleFn(msg)
}
