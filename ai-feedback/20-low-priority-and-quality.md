# 20 — Nhóm Low / chất lượng code

- **Mức độ:** 🟢 Low
- **Vùng:** Nhiều
- **Trạng thái tổng:** ⬜ Chưa xử lý

Mỗi mục là một vấn đề nhỏ độc lập; có ô trạng thái riêng để bạn triage.

---

## 20.1 — Cache stampede + nuốt lỗi `Set` trong `CacheThis`
**Trạng thái:** ⬜
[provider/cache-provider/cache_this.go](../core/../provider/cache-provider/cache_this.go)

Không có single-flight per-key: nhiều miss cùng key gọi `fetchFn` song song (stampede). `store.Set` chạy trong `go` tách rời, lỗi không bao giờ được quan sát.
→ Dùng `golang.org/x/sync/singleflight` keyed theo `cacheKey`; log lỗi `Set`.

---

## 20.2 — `fmt.Println`/`fmt.Printf` trong code production
**Trạng thái:** ⬜
16 chỗ, ví dụ:
- [server/gobwas/1_server.go `PrintStats`](../core/service/websocket/server/gobwas/1_server.go)
- [server/gobwas/2_frame_handle.go `handleProtocolError`/`handleContinuationFrame`](../core/service/websocket/server/gobwas/2_frame_handle.go)
- [connection-manager `PrintStats`](../core/service/websocket/connection-manager/connection_mamanger.go)

→ Thay bằng `slog` (có level, structured, tôn trọng cấu hình log). Không nên in ra stdout trong thư viện.

---

## 20.3 — `X-Forwarded-Proto` tin tưởng vô điều kiện
**Trạng thái:** ⬜
[mediator/delivery/1.issue_tmp_token.go](../core/service/websocket/mediator/delivery/1.issue_tmp_token.go) (dòng 53-54)

```go
protocolHeader := r.Header.Get("x-forwarded-proto")
cookieSecure := protocolHeader == "https"
```
Header do client kiểm soát (trừ khi edge proxy đảm bảo strip/overwrite). Client có thể ép `Secure` sai.
→ Suy scheme từ `r.TLS != nil`, hoặc chỉ tin header sau khi xác thực peer là proxy của mình.

---

## 20.4 — Fragmented frame xử lý sai
**Trạng thái:** ⬜
[server/gobwas/2_frame_handle.go](../core/service/websocket/server/gobwas/2_frame_handle.go) (dòng 89-96, 121-126)

Frame text/binary không-`fin` và continuation frame đều bị coi như message hoàn chỉnh (continuation chỉ `Printf` rồi bỏ). Nếu một client (không phải SDK chuẩn) gửi message phân mảnh → **hỏng/mất dữ liệu**.
→ Ghép fragment đúng chuẩn RFC 6455, hoặc đóng connection với protocol error nếu không hỗ trợ phân mảnh (rõ ràng hơn là im lặng xử lý sai).

---

## 20.5 — Cleanup DynamoDB dùng full-table `Scan` + filter
**Trạng thái:** ⬜
[impl-dynamodb/active_conn/cleanup_expired_connections.go](../core/repository/impl-dynamodb/active_conn/cleanup_expired_connections.go)

`Scan` đọc toàn bảng rồi mới lọc `TTL < now` → tốn RCU tuyến tính theo tổng số item, đắt ở quy mô lớn.
→ Cân nhắc GSI theo TTL/bucket thời gian để `Query` thay vì `Scan`; hoặc bật DynamoDB native TTL (lưu ý: native TTL cần epoch **giây**, trong khi hiện lưu **milli** — sẽ cần chuyển đổi nếu bật).

---

## 20.6 — Connection/FD leak khi `onNewStuff.Do` fail sau khi đã tạo connection
**Trạng thái:** ⬜
[mediator/delivery/2.gobwas_endpoint.go](../core/service/websocket/mediator/delivery/2.gobwas_endpoint.go) (dòng 45-56)

Sau `NewConnection` thành công (đã đăng ký fd vào netpoll), nếu `onNewStuff.Do(wsConn)` lỗi (vd DB `AddConnection` fail) → handler trả 500 nhưng **không** `wsConn.Close()` → socket không vào ConnectionManager, không được dọn → rò FD dưới sự cố DB kéo dài.
→ Trên nhánh lỗi, gọi `wsConn.Close()` trước khi return.

---

## 20.7 — Độ phủ test thấp
**Trạng thái:** ⬜
12 file `_test.go` / 303 file Go. Không có integration test chạy Postgres/DynamoDB thật (test migration chỉ kiểm logic Go thuần).
→ Bổ sung integration test (repo có sẵn skill `golang-unit-test-generator` + testcontainers) cho: rate limiter (bắt deadlock #01), repository parity (#04, #18), migration chạy DB thật, pubsub reconnect (#10).

---

## 20.8 — Singleton `server`/`once` trong gobwas
**Trạng thái:** ⬜
[server/gobwas/1_server.go](../core/service/websocket/server/gobwas/1_server.go) (dòng 35-76)

`NewServer` dùng `sync.Once` + biến package-level `server`. Lần gọi thứ hai **bỏ qua tham số mới** và trả server cũ → footgun cho test/multi-instance/hot-reload config.
→ Cân nhắc bỏ singleton, để DI quản lý vòng đời một instance.

---

## Ghi chú review chung
> _(chỗ trống để bạn ghi quyết định cho cả nhóm)_
