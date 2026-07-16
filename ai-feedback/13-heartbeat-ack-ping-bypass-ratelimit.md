# 13 — Heartbeat/Ack/ping bỏ qua rate limit (DoS)

- **Mức độ:** 🟡 Medium
- **Vùng:** Security / DoS
- **Trạng thái:** ⬜ Chưa xử lý
- **File liên quan:**
  - [core/service/websocket/client-msg-handler/0_main_handler.go](../core/service/websocket/client-msg-handler/0_main_handler.go) (`handleMessage`, switch 93-129)
  - [core/service/websocket/server/gobwas/2_frame_handle.go](../core/service/websocket/server/gobwas/2_frame_handle.go) (`handlePingFrame`)

## Mô tả

Trong `handleMessage`, kiểm tra rate-limit (`rateLimiter.Get(auth).Allow()`) **chỉ nằm ở nhánh `default`**. `MessageTypeHeartbeat` và `MessageTypeAck` `return` trước khi tới rate limiter.

→ Client có thể flood tùy ý message heartbeat/ack (mỗi frame tới 1MB theo `MaxFrameSize`) mà **không bị giới hạn**. Mỗi frame vẫn tốn một `workerPool.Submit`, ghi DB heartbeat (có throttle riêng nhưng vẫn tốn CPU/parse), v.v.

Ngoài ra `handlePingFrame` **echo pong vô điều kiện** cho mọi ping frame nhận được, cũng không rate-limit → flood ping → flood pong.

→ Một client độc hại làm cạn worker pool dùng chung, ảnh hưởng mọi connection trên container.

## Đề xuất sửa

- Áp cùng rate limiter (hoặc một frame-level limiter nhẹ) cho heartbeat/ack, không chỉ nhánh `default`.
- Rate-limit ping/pong ở tầng frame (giới hạn số control frame mỗi giây per-connection).

## Ghi chú review
> _(chỗ trống để bạn ghi quyết định)_
