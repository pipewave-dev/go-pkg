# 15 — Cửa sổ mất message ở msg-hub `Consume` (GetAll → DeleteAll không nguyên tử)

- **Mức độ:** 🟡 Medium
- **Vùng:** Messaging / pending-message
- **Trạng thái:** ✅ Đã xử lý
- **File liên quan:**
    - [core/service/websocket/msg-hub/service.go](../core/service/websocket/msg-hub/service.go) (`Consume`, dòng 142-157; `DeleteAllPendingMessage`)
    - [core/service/websocket/mediator/delivery/0.new.go](../core/service/websocket/mediator/delivery/0.new.go) (gọi `Consume` lúc `onNew`, dòng ~195)

## Mô tả

```go
func (s *msgHubSvc) Consume(ctx, userID, instanceID) ([][]byte, error) {
    msgs, aErr := s.repo.GetAll(ctx, userID, instanceID)   // (1) đọc tất cả
    ...
    s.DeleteAllPendingMessage(ctx, userID, instanceID)     // (2) xoá TẤT CẢ
    return msgs, nil
}
```

`GetAll` rồi `DeleteAll` **không nguyên tử**. Giữa (1) và (2), một container khác có thể `Save` thêm pending message mới cho cùng `(userID, instanceID)` (ví dụ `SendToSession` khi session đang temp-disconnect). `DeleteAll` xoá **toàn bộ** pending, bao gồm message mới đó → message bị **xoá mà không giao** → mất message.

Cửa sổ hẹp nhưng có thật trong luồng reconnect nhiều container.

## Đề xuất sửa (chọn 1)

- Xoá theo **danh sách ID/khoá đã đọc** ở (1) thay vì `DeleteAll` (chỉ xoá đúng những gì đã consume).
- Hoặc dùng thao tác atomic "read-and-delete" của backend (ví dụ trả về + xoá theo điều kiện `send_at <= tReadMax`).
- Hoặc chấp nhận at-least-once và để client dedup theo message ID (đã có `deduplicator`), nhưng cần đảm bảo không **mất** (hiện là mất, không phải trùng).

## Ghi chú review

- msgHub là nơi lưu các message vào buffer khi 1 websocket connection có header `X-Pipewave-InstanceID` bị vấn đề. Khi Websocket reconnect thì message từ msgHub sẽ được gửi về lại websocket connection
    - do đó `instanceID` trong các method trên là unique, và getAll rồi deleteAll có thể đảm bảo atomic (nếu code không chạy song song)
