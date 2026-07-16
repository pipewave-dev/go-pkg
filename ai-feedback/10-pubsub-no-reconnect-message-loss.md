# 10 — Pub/Sub `Subscribe` không reconnect → mất message âm thầm

- **Mức độ:** 🟠 High
- **Vùng:** Pubsub / messaging
- **Trạng thái:** ⬜ Chưa xử lý
- **File liên quan:**
  - [pkg/pubsub/adapters/valkey/instance.go](../pkg/pubsub/adapters/valkey/instance.go) (`Subscribe`, ~dòng 50-78)
  - [pkg/pubsub/adapters/redis/instance.go](../pkg/pubsub/adapters/redis/instance.go) (bản redis tương tự)

## Mô tả

```go
go func() {
    ...
    err = va.coreValkey.Receive(subCtx, ..., func(msg valkey.PubSubMessage) {...})
    if err != nil && subCtx.Err() == nil {
        slog.Error("Valkey subscription error", slog.Any("err", err))
    }
}()
```

Nếu `Receive` trả lỗi vì lý do khác context-cancel (blip mạng, server restart...), goroutine chỉ **log một lần rồi thoát** — không có vòng reconnect/resubscribe. Từ đó `handler` **không bao giờ được gọi lại** cho channel đó, dù caller chưa gọi `unsubscribe` và **không có cách nào biết** subscription đã chết.

→ Đây là kênh định tuyến message giữa các container (broadcast). Một blip Valkey/Redis thoáng qua **giết subscription vĩnh viễn** cho tới khi restart process → mất message thầm lặng.

## Đề xuất sửa

- Bọc `Receive` trong vòng retry/backoff cho tới khi `subCtx` bị hủy:
```go
go func() {
    for subCtx.Err() == nil {
        err := va.coreValkey.Receive(subCtx, ...)
        if subCtx.Err() != nil { return }
        slog.Error("subscription dropped, retrying", slog.Any("err", err))
        time.Sleep(backoff.Next()) // exponential + jitter
    }
}()
```
- Hoặc phát tín hiệu "subscription chết" (callback/health) để caller chủ động resubscribe.
- Cân nhắc cảnh báo/metric khi retry để không im lặng.

## Ghi chú review
> _(chỗ trống để bạn ghi quyết định)_
