# 07 — `actx`: mutex khai báo nhưng không bao giờ khoá → data race

- **Mức độ:** 🟠 High
- **Vùng:** Shared / context
- **Trạng thái:** ✅ Đã sửa
- **File liên quan:**
    - [shared/actx/actx.go](../shared/actx/actx.go) (field `m sync.Mutex` trong `alterData`)
    - [shared/actx/auth.go](../shared/actx/auth.go), [shared/actx/trace-id.go](../shared/actx/trace-id.go), [shared/actx/user-ip.go](../shared/actx/user-ip.go), [shared/actx/user-agent.go](../shared/actx/user-agent.go), [shared/actx/broadcast.go](../shared/actx/broadcast.go)

## Mô tả

`alterData` có sẵn `m sync.Mutex` rõ ràng để bảo vệ `traceId`, `wsAuth`, `fromBroadcast`, `userIp`, `userAgent`. Nhưng **không** getter/setter nào gọi `Lock()`/`Unlock()` (grep = 0 kết quả).

Cùng một `*alterData` được chia sẻ qua `context.Context` (`ctx.Value(privKey)`), và context được truyền vào các handler frame chạy song song / worker pool. Các lời gọi `SetWebsocketAuth`/`SetTraceID`/`GetTraceID` đồng thời trên cùng context → **data race** trên string/struct thuần (phát hiện được bằng `go test -race`), có thể sinh trace ID lộn xộn hoặc `WebsocketAuth` hỏng.

## Đề xuất sửa

Thực sự khoá quanh mọi đọc/ghi field, ví dụ:

```go
func (a *aContext) SetWebsocketAuth(auth voAuth.WebsocketAuth) {
    a.data.m.Lock()
    defer a.data.m.Unlock()
    a.data.wsAuth = auth
}
func (a *aContext) GetWebsocketAuth() voAuth.WebsocketAuth {
    a.data.m.Lock()
    defer a.data.m.Unlock()
    return a.data.wsAuth
}
```

Áp dụng nhất quán cho toàn bộ setter/getter trong package. Cân nhắc `sync.RWMutex` nếu đọc nhiều hơn ghi.
