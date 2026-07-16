# 11 — Trace ID không propagate qua middleware

- **Mức độ:** 🟡 Medium
- **Vùng:** Observability / middleware
- **Trạng thái:** ✅ Đã xử lý
- **File liên quan:**
    - [pkg/mux-middleware/request_id.go](../pkg/mux-middleware/request_id.go) (`RequestID`)
    - [core/delivery/module/2.2.middleware.go](../core/delivery/module/2.2.middleware.go) (callback dòng 15-19, 34-38)
    - [shared/actx/actx.go](../shared/actx/actx.go) (`From`)

## Mô tả

Middleware dùng ctx trả về từ callback:

```go
ctx := r.Context()
if callbackFn != nil { ctx = callbackFn(ctx, rid) }
next.ServeHTTP(w, r.WithContext(ctx))
```

Nhưng callback lại trả về `ctx` **gốc**:

```go
m.mw.RequestID(func(ctx context.Context, rId string) context.Context {
    aCtx := actx.From(ctx)   // nếu ctx chưa có alterData: tạo alterData mới + ctx MỚI mang value
    aCtx.SetTraceID(rId)     // set lên alterData mới
    return ctx               // <-- trả về ctx GỐC, KHÔNG mang alterData vừa tạo
})
```

Khi `alterData` chưa tồn tại, `actx.From` tạo alterData mới và gắn vào **một ctx mới** (`context.WithValue`), set traceID lên đó, nhưng callback trả về **ctx gốc** (không có value). Downstream `actx.From(r.Context())` không thấy alterData → tạo lại mới → **mất trace ID** → log/observability không correlate được request.

## Đề xuất sửa

Trả về context mang alterData (`aCtx` là `context.Context` nhờ embedding):

```go
m.mw.RequestID(func(ctx context.Context, rId string) context.Context {
    aCtx := actx.From(ctx)
    aCtx.SetTraceID(rId)
    return aCtx   // hoặc aCtx.Context
})
```

Áp dụng cho cả `AllMiddilewares` và `WsMiddlewares`.
