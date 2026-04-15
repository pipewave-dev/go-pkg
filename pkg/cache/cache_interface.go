package cache

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"time"

	"github.com/samber/lo"
	"github.com/vmihailenco/msgpack/v5"
)

type NetworkSerializable interface {
	Encode() []byte
	Decode([]byte) error
}

type CacheProvider interface {
	Set(ctx context.Context, key string, value any, ttl time.Duration) (setable bool)
	Get(ctx context.Context, key string, unmarshalTo any) (found bool)
	Del(ctx context.Context, key string) (deleted bool)
	Incr(ctx context.Context, key string) bool
	Decr(ctx context.Context, key string) bool

	Flush() error
}

type cacheProvider struct {
	store StoreAdapter
}

func (r *cacheProvider) Incr(ctx context.Context, key string) bool {
	return r.store.Incr(ctx, key)
}

func (r *cacheProvider) Decr(ctx context.Context, key string) bool {
	return r.store.Decr(ctx, key)
}

func New(store StoreAdapter) CacheProvider {
	ins := &cacheProvider{
		store,
	}
	return ins
}

func convertNumberToStr(v any) string {
	switch v.(type) {
	case int:
		return strconv.Itoa(v.(int))
	case int64:
		return strconv.FormatInt(v.(int64), 10)
	case int8:
		return strconv.FormatInt(int64(v.(int8)), 10)
	case int16:
		return strconv.FormatInt(int64(v.(int16)), 10)
	case int32:
		return strconv.FormatInt(int64(v.(int32)), 10)
	case uint:
		return strconv.FormatUint(uint64(v.(uint)), 10)
	case uint8:
		return strconv.FormatUint(uint64(v.(uint8)), 10)
	case uint16:
		return strconv.FormatUint(uint64(v.(uint16)), 10)
	case uint32:
		return strconv.FormatUint(uint64(v.(uint32)), 10)
	case uint64:
		return strconv.FormatUint(v.(uint64), 10)
	case float32:
		return strconv.FormatFloat(float64(v.(float32)), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(v.(float64), 'f', -1, 64)
	}

	return ""
}

func convertStrToNumber[T any](v string) (T, error) {
	var zero T
	switch any(zero).(type) {
	case int:
		i, err := strconv.Atoi(v)
		if err != nil {
			return zero, err
		}
		return any(i).(T), nil
	case int64:
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return zero, err
		}
		return any(i).(T), nil
	case int8:
		i, err := strconv.ParseInt(v, 10, 8)
		if err != nil {
			return zero, err
		}
		return any(int8(i)).(T), nil
	case int16:
		i, err := strconv.ParseInt(v, 10, 16)
		if err != nil {
			return zero, err
		}
		return any(int16(i)).(T), nil
	case int32:
		i, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return zero, err
		}
		return any(int32(i)).(T), nil
	case uint:
		u, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return zero, err
		}
		return any(uint(u)).(T), nil
	case uint8:
		u, err := strconv.ParseUint(v, 10, 8)
		if err != nil {
			return zero, err
		}
		return any(uint8(u)).(T), nil
	case uint16:
		u, err := strconv.ParseUint(v, 10, 16)
		if err != nil {
			return zero, err
		}
		return any(uint16(u)).(T), nil
	case uint32:
		u, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return zero, err
		}
		return any(uint32(u)).(T), nil
	case uint64:
		u, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return zero, err
		}
		return any(u).(T), nil
	case float32:
		f, err := strconv.ParseFloat(v, 32)
		if err != nil {
			return zero, err
		}
		return any(float32(f)).(T), nil
	case float64:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return zero, err
		}
		return any(f).(T), nil
	default:
		return zero, fmt.Errorf("unsupported type: %T", zero)
	}
}

func (r *cacheProvider) Set(ctx context.Context, key string, value any, ttl time.Duration) (setable bool) {
	switch v := value.(type) {
	case int, int64, int8, int16, int32, uint, uint8, uint16, uint32, uint64, float32, float64:
		return r.store.Set(ctx, key, convertNumberToStr(v), ttl)
	case string:
		return r.store.Set(ctx, key, v, ttl)
	case []byte:
		return r.store.Set(ctx, key, string(v), ttl)
	case NetworkSerializable:
		valb := v.Encode()
		return r.store.Set(ctx, key, string(valb), ttl)
	default:
		if lo.IsNil(value) {
			return r.store.Set(ctx, key, "", ttl)
		}
		// if value is not string, then marshal it using msgpack
		valb, err := msgpack.Marshal(value)
		if err != nil {
			slog.ErrorContext(ctx, "msgpack.Marshal", slog.Any("err", err))
			return false
		}
		return r.store.Set(ctx, key, string(valb), ttl)
	}
}

func (r *cacheProvider) Get(ctx context.Context, key string, unmarshalTo any) (found bool) {
	vv := reflect.ValueOf(unmarshalTo)
	if vv.Kind() != reflect.Pointer {
		slog.ErrorContext(ctx, "unmarshalTo type must be Pointer")
		return false
	}
	if vv.IsNil() {
		slog.ErrorContext(ctx, "unmarshalTo must not nil")
		return false
	}

	var (
		valueStr string
		err      error
	)
	valueStr, found = r.store.Get(ctx, key)
	if !found {
		return false
	}

	switch u := unmarshalTo.(type) {
	case *int:
		v, err := convertStrToNumber[int](valueStr)
		if err != nil {
			slog.ErrorContext(ctx, "convertStrToNumber", slog.Any("err", err))
			return false
		}
		*u = v
		return true
	case *int8:
		v, err := convertStrToNumber[int8](valueStr)
		if err != nil {
			slog.ErrorContext(ctx, "convertStrToNumber", slog.Any("err", err))
			return false
		}
		*u = v
		return true
	case *int16:
		v, err := convertStrToNumber[int16](valueStr)
		if err != nil {
			slog.ErrorContext(ctx, "convertStrToNumber", slog.Any("err", err))
			return false
		}
		*u = v
		return true
	case *int32:
		v, err := convertStrToNumber[int32](valueStr)
		if err != nil {
			slog.ErrorContext(ctx, "convertStrToNumber", slog.Any("err", err))
			return false
		}
		*u = v
		return true
	case *int64:
		v, err := convertStrToNumber[int64](valueStr)
		if err != nil {
			slog.ErrorContext(ctx, "convertStrToNumber", slog.Any("err", err))
			return false
		}
		*u = v
		return true
	case *uint:
		v, err := convertStrToNumber[uint](valueStr)
		if err != nil {
			slog.ErrorContext(ctx, "convertStrToNumber", slog.Any("err", err))
			return false
		}
		*u = v
		return true
	case *uint8:
		v, err := convertStrToNumber[uint8](valueStr)
		if err != nil {
			slog.ErrorContext(ctx, "convertStrToNumber", slog.Any("err", err))
			return false
		}
		*u = v
		return true
	case *uint16:
		v, err := convertStrToNumber[uint16](valueStr)
		if err != nil {
			slog.ErrorContext(ctx, "convertStrToNumber", slog.Any("err", err))
			return false
		}
		*u = v
		return true
	case *uint32:
		v, err := convertStrToNumber[uint32](valueStr)
		if err != nil {
			slog.ErrorContext(ctx, "convertStrToNumber", slog.Any("err", err))
			return false
		}
		*u = v
		return true
	case *uint64:
		v, err := convertStrToNumber[uint64](valueStr)
		if err != nil {
			slog.ErrorContext(ctx, "convertStrToNumber", slog.Any("err", err))
			return false
		}
		*u = v
		return true
	case *float32:
		v, err := convertStrToNumber[float32](valueStr)
		if err != nil {
			slog.ErrorContext(ctx, "convertStrToNumber", slog.Any("err", err))
			return false
		}
		*u = v
		return true
	case *float64:
		v, err := convertStrToNumber[float64](valueStr)
		if err != nil {
			slog.ErrorContext(ctx, "convertStrToNumber", slog.Any("err", err))
			return false
		}
		*u = v
		return true
	case *string:
		// if unmarshalTo is string, then return the value as string
		*u = valueStr
		return true
	case *[]byte:
		// if unmarshalTo is []byte, then return the value as []byte
		*u = []byte(valueStr)
		return true
	case NetworkSerializable:
		err = u.Decode([]byte(valueStr))
		if err != nil {
			slog.ErrorContext(ctx, "NetworkSerializable.Decode", slog.Any("err", err))
			return false
		}
		return true
	default:
		if valueStr == "" {
			return true
		}
		// if unmarshalTo is not string, then unmarshal it using msgpack
		err = msgpack.Unmarshal([]byte(valueStr), unmarshalTo)
		if err != nil {
			slog.ErrorContext(ctx, "msgpack.Unmarshal", slog.Any("err", err))
			return false
		}
		return true
	}
}

func (r *cacheProvider) Del(ctx context.Context, key string) (cleared bool) {
	return r.store.Del(ctx, key)
}

func (r *cacheProvider) Flush() error {
	return r.store.Flush()
}
