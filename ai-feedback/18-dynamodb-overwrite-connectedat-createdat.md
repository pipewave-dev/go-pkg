# 18 — DynamoDB ghi đè `ConnectedAt`/`CreatedAt` khi reconnect (lệch với Postgres)

- **Mức độ:** 🟡 Medium
- **Vùng:** Repository (parity) / metrics
- **Trạng thái:** ⬜ Chưa xử lý
- **File liên quan:**
  - [core/repository/impl-dynamodb/active_conn/exprbuilder/0.2.creator.go](../core/repository/impl-dynamodb/active_conn/exprbuilder/0.2.creator.go) (`Create` → `AddConnection`, `ConnectedAt: now`)
  - [core/repository/impl-dynamodb/user/exprbuilder/0.2.creator.go](../core/repository/impl-dynamodb/user/exprbuilder/0.2.creator.go) (`Upsert`, `CreatedAt: now`)
  - So sánh: [impl-postgres/active_conn/add_connection.go](../core/repository/impl-postgres/active_conn/add_connection.go), [impl-postgres/user/upsert.go](../core/repository/impl-postgres/user/upsert.go)

## Mô tả

DynamoDB dùng `PutItem` không điều kiện, **luôn** set `ConnectedAt: now` / `CreatedAt: now` → ghi đè hoàn toàn item cũ khi reconnect cùng `(UserID, InstanceID)` / cùng user.

Postgres cố ý **giữ nguyên**:
- `add_connection.go`: `ON CONFLICT (user_id, instance_id) DO UPDATE SET holder_id, connection_type, status, last_heartbeat, ttl` — **không** đụng `connected_at`.
- `upsert.go`: `ON CONFLICT (id) DO UPDATE SET last_heartbeat = $2` — **không** đụng `created_at`.

→ Cùng interface, hai backend cho kết quả khác nhau: trên DynamoDB, mỗi reconnect/upsert reset `ConnectedAt`/`CreatedAt` về "now" → **sai metric session-duration / user-tenure** chỉ ở backend DynamoDB.

## Đề xuất sửa

Dùng `UpdateItem` với `Set` bỏ `ConnectedAt`/`CreatedAt`, hoặc set qua `if_not_exists(...)` (set khi insert, giữ khi update):

```go
update := expression.
    Set(expression.Name(FieldConnectedAt),
        expression.IfNotExists(expression.Name(FieldConnectedAt), expression.Value(nowMilli))).
    Set(...) // các field khác cập nhật bình thường
```

## Ghi chú review
> _(chỗ trống để bạn ghi quyết định)_
