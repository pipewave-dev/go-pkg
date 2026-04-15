package queue

import (
	"context"
	"log/slog"
	"reflect"
)

type NetworkSerializable interface {
	Encode() []byte
	Decode([]byte) error
}

type QueueProvider[ChannelT ~string, MsgT NetworkSerializable] interface {
	Publish(ctx context.Context, channel ChannelT, message MsgT) error
	FetchOne(ctx context.Context, channel string) (message MsgT, err error)
	FetchMany(ctx context.Context, channel string, maxItem int) (message []MsgT, err error)
	Len(ctx context.Context, channel string) (int64, error)
	Healthcheck(ctx context.Context) error

	Close()
}

type queueProvider[ChannelT ~string, MsgT NetworkSerializable] struct {
	adapter Adapter
}

func New[ChannelT ~string, MsgT NetworkSerializable](adapter Adapter) QueueProvider[ChannelT, MsgT] {
	ins := &queueProvider[ChannelT, MsgT]{
		adapter,
	}
	return ins
}

func (r *queueProvider[ChannelT, MsgT]) Publish(ctx context.Context, channel ChannelT, message MsgT) error {
	return r.adapter.Publish(ctx, string(channel), message.Encode())
}

func (r *queueProvider[ChannelT, MsgT]) FetchOne(ctx context.Context, channel string) (message MsgT, err error) {
	var m []byte
	m, err = r.adapter.FetchOne(ctx, channel)
	if err != nil {
		slog.Error("Queue FetchOne Adapter.FetchOne", slog.Any("err", err))
		return message, err
	}

	message, err = decodeNetworkSerializable[MsgT](m)
	if err != nil {
		slog.Error("Queue FetchOne NetworkSerializable.Decode", slog.Any("err", err))
		return message, err
	}
	return message, nil
}

func (r *queueProvider[ChannelT, MsgT]) FetchMany(ctx context.Context, channel string, maxItem int) (message []MsgT, err error) {
	var data [][]byte
	data, err = r.adapter.FetchMany(ctx, channel, maxItem)
	if err != nil {
		slog.Error("Queue FetchMany Adapter.FetchMany", slog.Any("err", err))
		return nil, err
	}

	message = make([]MsgT, 0, len(data))
	for _, d := range data {
		var msg MsgT
		msg, err = decodeNetworkSerializable[MsgT](d)
		if err != nil {
			slog.Error("Queue FetchMany NetworkSerializable.Decode", slog.Any("err", err))
			return message, err
		}
		message = append(message, msg)
	}

	return message, nil
}

func (r *queueProvider[ChannelT, MsgT]) Len(ctx context.Context, channel string) (int64, error) {
	return r.adapter.Len(ctx, channel)
}

func (r *queueProvider[ChannelT, MsgT]) Close() {
	r.adapter.Close()
}

func (r *queueProvider[ChannelT, MsgT]) Healthcheck(ctx context.Context) error {
	return r.adapter.Healthcheck(ctx)
}

type HandlerFn[T NetworkSerializable] = func(hdlMsg T)

func handleConverter[T NetworkSerializable](
	message []byte,
	handleFn HandlerFn[T],
) {
	msg, err := decodeNetworkSerializable[T](message)
	if err != nil {
		slog.Error("Queue handleConverter NetworkSerializable.Decode", slog.Any("err", err))
		return
	}
	handleFn(msg)
}

func decodeNetworkSerializable[T NetworkSerializable](data []byte) (T, error) {
	var (
		msg   T
		zeroT T
	)
	if reflect.TypeOf(msg).Kind() == reflect.Ptr {
		msg = reflect.New(reflect.TypeOf(msg).Elem()).Interface().(T)
	} else {
		msg = reflect.Zero(reflect.TypeOf(msg)).Interface().(T)
	}
	err := msg.Decode(data)
	if err != nil {
		return zeroT, err
	}
	return msg, nil
}
