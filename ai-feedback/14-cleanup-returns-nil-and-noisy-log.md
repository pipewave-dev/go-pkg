# 14 — `CleanUp` luôn trả `nil` + luôn log Error

- **Mức độ:** 🟡 Medium
- **Vùng:** Error handling
- **Trạng thái:** ⬜ Chưa xử lý
- **File liên quan:**
  - [core/delivery/module/3.get_service.go](../core/delivery/module/3.get_service.go) (`CleanUp`, dòng 95-108)
  - [shared/aerror](../shared/aerror) (`Append`, `AMultiError`)

## Mô tả

```go
func (g *getServices) CleanUp(ctx context.Context) aerror.AError {
    var multiErr aerror.AMultiError
    err1 := g.repo.ActiveConnStore().CleanUpExpiredConnections(ctx)
    err2 := g.repo.PendingMessage().CleanUpExpiredPendingMessages(ctx)
    aerror.Append(multiErr, err1, err2)   // (1) giá trị trả về bị bỏ
    slog.ErrorContext(ctx, "Failed to clean up expired websocket resources", ...) // (2) luôn log Error
    return nil                             // (3) luôn trả nil
}
```

Hai lỗi:
1. `aerror.Append` trả về `AMultiError` **mới** (không mutate `multiErr` truyền theo giá trị/interface nil). Kết quả bị vứt → `multiErr` mãi `nil`, hàm **luôn trả `nil`** → caller (cron cleanup) tưởng **luôn thành công** dù cả hai repo call fail.
2. `slog.ErrorContext` chạy **vô điều kiện** → log "Failed to clean up..." trên **mọi** lần cleanup thành công → log rác, gây nhiễu/cảnh báo giả.

## Đề xuất sửa

```go
func (g *getServices) CleanUp(ctx context.Context) aerror.AError {
    var multiErr aerror.AMultiError
    err1 := g.repo.ActiveConnStore().CleanUpExpiredConnections(ctx)
    err2 := g.repo.PendingMessage().CleanUpExpiredPendingMessages(ctx)
    multiErr = aerror.Append(multiErr, err1, err2)   // gán lại
    if multiErr != nil {
        slog.ErrorContext(ctx, "Failed to clean up expired websocket resources",
            slog.Any("activeConnError", err1),
            slog.Any("pendingMessageError", err2))
    }
    return multiErr
}
```

> Gợi ý: kiểm tra các call site khác của `aerror.Append` xem có bị cùng lỗi "bỏ giá trị trả về" không.

## Ghi chú review
> _(chỗ trống để bạn ghi quyết định)_
