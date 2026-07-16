# 01 — Self-deadlock trong `RateLimiter.Get()`

- **Mức độ:** 🔴 Critical
- **Vùng:** Concurrency / WebSocket
- **Trạng thái:** ✅ Đã xử lý
- **File liên quan:**
    - [core/service/websocket/rate-limiter/rate_limiter.go](../core/service/websocket/rate-limiter/rate_limiter.go) (`Get`, `New`)
    - [core/service/websocket/client-msg-handler/0_main_handler.go](../core/service/websocket/client-msg-handler/0_main_handler.go) (call site `rateLimiter.Get`)
    - [core/service/websocket/mediator/delivery/0.new.go](../core/service/websocket/mediator/delivery/0.new.go) (`rateLimiter.New`, `rateLimiter.Remove`)

## Mô tả

`Get()` giữ read-lock rồi khi miss lại gọi `New()` vốn lấy write-lock trên **cùng** mutex:

```go
func (r *rateLimiter) Get(auth voAuth.WebsocketAuth) *rate.Limiter {
    r.mu.RLock()
    defer r.mu.RUnlock()      // RUnlock chỉ chạy khi return
    ...
    rate, ok = r.userLimiter[auth.UserID]
    if ok { return rate }
    return r.New(auth)        // New() gọi r.mu.Lock() TRONG KHI RLock còn giữ
}

func (r *rateLimiter) New(auth voAuth.WebsocketAuth) *rate.Limiter {
    r.mu.Lock()               // <-- deadlock
    defer r.mu.Unlock()
    ...
}
```

`sync.RWMutex` **không reentrant**. Gọi `Lock()` khi cùng goroutine đang giữ `RLock()` → chờ vô hạn. Tệ hơn: theo cơ chế chống writer-starvation của Go, một writer đang pending sẽ **chặn mọi `RLock()` mới** → toàn bộ rate limiter treo cho mọi connection → **mất dịch vụ, phải restart process**.

## Kịch bản lỗi

`rateLimiter.New(auth)` được gọi ở `onNew` ([0.new.go:191](../core/service/websocket/mediator/delivery/0.new.go#L191)), nhưng có 2 đường kích hoạt `Get` khi entry chưa/không còn tồn tại:

1. **Ngay sau upgrade:** connection đã đăng ký netpoll và bắt đầu submit read-event **trước** khi `onNewStuff.Do()` (gọi `New`) chạy xong. Client gửi message đầu tiên tức thì → `Get(auth)` miss → `New()` → deadlock.
2. **Race lúc disconnect:** `onClose` gọi `Remove(auth)` xoá limiter, nhưng một message in-flight trong worker pool vẫn gọi `Get(auth)` → miss → `New()` → deadlock. Goroutine worker treo vĩnh viễn; lặp lại nhiều lần → cạn worker pool → outage.

## Đề xuất sửa

Không lồng write-lock trong read-lock. Double-checked lookup:

```go
func (r *rateLimiter) Get(auth voAuth.WebsocketAuth) *rate.Limiter {
    r.mu.RLock()
    lim, ok := r.lookup(auth) // đọc thuần, không gọi New
    r.mu.RUnlock()
    if ok {
        return lim
    }
    return r.New(auth) // New tự lấy write-lock, không còn giữ RLock
}
```

Bổ sung: `New` nên trả về limiter đã tồn tại nếu có (idempotent) để tránh ghi đè khi 2 goroutine cùng tạo.

## Action:

- Đã sửa lại code
    - sửa lại thành `pipewave-gopkg/core/service/websocket/rate-limiter/rate_limiter.go` line 76~93
