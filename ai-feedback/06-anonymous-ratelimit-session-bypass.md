# 06 — Bypass rate-limit & chiếm session anonymous qua header client `X-Pipewave-ID`

- **Mức độ:** 🟠 High
- **Vùng:** Security / rate-limit / connection identity
- **Trạng thái:** ⬜ Chưa xử lý
- **File liên quan:**
  - [core/service/websocket/mediator/delivery/1.issue_tmp_token.go](../core/service/websocket/mediator/delivery/1.issue_tmp_token.go) (đọc `X-Pipewave-ID`)
  - [core/service/websocket/rate-limiter/rate_limiter.go](../core/service/websocket/rate-limiter/rate_limiter.go) (key anonymous = `InstanceID`)
  - [core/service/websocket/connection-manager/connection_mamanger.go](../core/service/websocket/connection-manager/connection_mamanger.go) (`anonymousConn[InstanceID]`)

## Mô tả

`InstanceID` của anonymous = header `X-Pipewave-ID` do **client tự đặt**, không xác thực/không ký, không ràng buộc IP hay token:

```go
instanceHeader := r.Header.Get("X-Pipewave-ID")
...
wsAuth = voAuth.AnonymousUserWebsocketAuthWithMetadata(instanceHeader, metadata)
```

Cả **rate limiter** (`anonymousLimiter[InstanceID]`) lẫn **connection map** (`anonymousConn[InstanceID]`) đều key theo giá trị này.

## Hai vector

### 6a. Bypass rate-limit anonymous
Đổi `X-Pipewave-ID` ngẫu nhiên mỗi lần `/issue-tmp-token` (hoặc mỗi lần reconnect `/gw`) → `rateLimiter.New()` cấp **bucket mới toanh** mỗi lần → **bỏ qua hoàn toàn** `ANONYMOUS_RATE`/`ANONYMOUS_BURST`. Cho phép flood message không giới hạn.

### 6b. Chiếm/đá session anonymous
Gửi `X-Pipewave-ID` **trùng** với session anonymous đang hoạt động của nạn nhân → (a) connection của nạn nhân bị đóng (`existingConn.Close()` khi register), và (b) message định tuyến tới instance đó bị attacker nhận. Vì ID không phải bí mật và không được server xác thực, đây là primitive chiếm session nếu ID đoán được/quan sát được.

## Đề xuất sửa

- Key rate-limit anonymous theo **IP** (hoặc IP + ID do server cấp), không theo header client.
- ID định danh instance dùng để key connection/queue nên do **server sinh** (trả về trong `Exchange`) hoặc ràng buộc vào `connTmpToken`/IP, để attacker không thể replay ID đã biết.
- Lưu ý phối hợp với [#12](12-conn-token-replay-and-url-leak.md).

## Ghi chú review
> _(chỗ trống để bạn ghi quyết định)_
