# 04 — DynamoDB `UpdateItem` thiếu điều kiện → "ghost record" + sai lệch với Postgres

- **Mức độ:** 🟠 High
- **Vùng:** Repository (parity DynamoDB ↔ Postgres) / data integrity
- **Trạng thái:** ✅ Đã sửa
- **File liên quan:**
    - [core/repository/impl-dynamodb/active_conn/exprbuilder/0.3.updater.go](../core/repository/impl-dynamodb/active_conn/exprbuilder/0.3.updater.go) (`UpdateLastHeartbeat`, `UpdateStatus`, `UpdateStatusTransferring`)
    - [core/repository/impl-dynamodb/user/exprbuilder/0.3.updater.go](../core/repository/impl-dynamodb/user/exprbuilder/0.3.updater.go) (`UpdateLastHeartbeat`)
    - So sánh: [impl-postgres/active_conn/update_heart_beat.go](../core/repository/impl-postgres/active_conn/update_heart_beat.go), [update_status.go](../core/repository/impl-postgres/active_conn/update_status.go), [update_status_transferring.go](../core/repository/impl-postgres/active_conn/update_status_transferring.go)
    - [shared/utils/repo-helper/dynamo_error_check.go](../shared/utils/repo-helper/dynamo_error_check.go) (helper có sẵn nhưng chưa dùng)

## Mô tả

Các `UpdateItemInput` của DynamoDB **không set `ConditionExpression`**. `UpdateItem` của DynamoDB là **upsert**: nếu key chưa tồn tại, nó **tạo item mới** chỉ gồm key + các field trong `Set()`, các field bắt buộc còn lại để rỗng/zero.

Trong khi bản Postgres là `UPDATE ... WHERE user_id=$x AND instance_id=$y` → **0 rows, no-op** an toàn khi record không tồn tại.

→ **Sai lệch hành vi giữa hai backend cùng interface**, đồng thời là lỗi data-integrity độc lập.

## Kịch bản lỗi

1. Session bị xoá qua `RemoveConnection` (xoá item DynamoDB).
2. Một heartbeat/`transferring` trễ hoặc trùng cho cùng `(UserID, InstanceID)` tới sau (thường gặp khi retry mạng/đảo thứ tự quanh lúc reconnect).
3. `UpdateHeartBeat`/`UpdateStatus`/`UpdateStatusTransferring` **tái tạo item thiếu field**: `Status=0` (unknown), `ConnectionType=0`, `HolderID=""`, `ConnectedAt` zero-time.
4. Riêng `UpdateStatus` **không set `TTL`** → ghost record **không có attribute TTL** → filter cleanup `expression.Name(FieldTTL).LessThan(...)` cho item thiếu TTL **luôn = false** → `CleanUpExpiredConnections` **không bao giờ xoá** nó → tồn tại vĩnh viễn, bị `GetActiveConnections`/`GetInstanceConnection`/`CountActiveConnections` trả về như connection sống → **định tuyến sai**.

Tương tự `userRepo.UpdateLastHeartbeat` có thể chế ra `User` không có `CreatedAt`.

## Đề xuất sửa

Thêm điều kiện tồn tại cho cả 3 method active_conn và user updater:

```go
cond := expression.AttributeExists(expression.Name(FieldUserID))
expr, _ := expression.NewBuilder().WithUpdate(update).WithCondition(cond).Build()
input := &dynamodb.UpdateItemInput{
    ...
    ConditionExpression: expr.Condition(),
}
```

Và map `ConditionalCheckFailedException` thành not-found (dùng [dynamo_error_check.go](../shared/utils/repo-helper/dynamo_error_check.go)) để khớp semantics no-op của Postgres.
