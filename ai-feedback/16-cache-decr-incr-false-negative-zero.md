# 16 — `Decr`/`Incr` trả false-negative khi counter về đúng 0

- **Mức độ:** 🟡 Medium
- **Vùng:** Cache / Valkey
- **Trạng thái:** ✅ Đã xử lý
- **File liên quan:**
    - [pkg/cache/adapters/valkey/decr.go](../pkg/cache/adapters/valkey/decr.go) (dòng ~11)
    - [pkg/cache/adapters/valkey/incr.go](../pkg/cache/adapters/valkey/incr.go)
    - [pkg/cache/cache_interface.go](../pkg/cache/cache_interface.go) (`Incr`/`Decr`)

## Mô tả

```go
m, err := c.Do(ctx, c.B().Decr().Key(key).Build()).AsBool()
```

Với reply số của Redis/Valkey, `valkey-go`'s `AsBool()` cài đặt `val = m.intlen != 0`. Nghĩa là một `DECR` thành công **về đúng 0** (rất phổ biến: counter/semaphore/refcount lock về 0) sẽ bị trả về `false` — **không phân biệt** được với lỗi Redis thật.

→ Caller dùng bool để quyết định thành/bại (retry, alert, business logic) sẽ hiểu nhầm một thao tác hợp lệ về 0 là **thất bại**.

## Đề xuất sửa

Dùng `.AsInt64()` và kiểm `err`:

```go
n, err := c.Do(ctx, c.B().Decr().Key(key).Build()).AsInt64()
if err != nil {
    return 0, false // hoặc trả lỗi
}
return n, true // trả về giá trị count, không đánh đồng 0 với lỗi
```

Điều chỉnh chữ ký `Incr`/`Decr` để trả về `(int64, error)` hoặc `(int64, bool)` thay vì chỉ `bool`.
