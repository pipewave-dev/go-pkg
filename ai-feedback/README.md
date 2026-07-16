# AI Code Review — `pipewave-gopkg`

Tổng hợp các vấn đề phát hiện qua review toàn bộ project (concurrency, repository parity, security, providers/shared).
Mỗi vấn đề có một file chi tiết riêng. Cột **Trạng thái** để bạn tự cập nhật khi review/sửa.

> Ghi chú: `go build ./...` pass. Các phát hiện đã được kiểm chứng chéo giữa nhiều hướng review.
> Đường dẫn file nguồn tính từ gốc module `pipewave-gopkg/`.

## Mục lục theo mức độ ưu tiên

| # | Vấn đề | Mức độ | Vùng | File | Trạng thái |
|---|--------|--------|------|------|-----------|
| 01 | Self-deadlock trong `RateLimiter.Get()` | 🔴 Critical | Concurrency | [01-ratelimiter-deadlock.md](01-ratelimiter-deadlock.md) | ⬜ Chưa xử lý |
| 02 | Vòng đọc netpoll edge-triggered: đọc thiếu + đọc đồng thời | 🟠 High | Concurrency | [02-netpoll-edge-triggered-read.md](02-netpoll-edge-triggered-read.md) | ⬜ Chưa xử lý |
| 03 | WorkerPool: `Submit` blocking + panic khi shutdown | 🟠 High | Concurrency | [03-workerpool-blocking-and-shutdown-panic.md](03-workerpool-blocking-and-shutdown-panic.md) | ✅ Đã sửa |
| 04 | DynamoDB `UpdateItem` thiếu điều kiện → ghost record | 🟠 High | Repository | [04-dynamodb-updateitem-ghost-records.md](04-dynamodb-updateitem-ghost-records.md) | ✅ Đã sửa |
| 05 | CORS regex không neo `^...$` | 🟠 High | Security | [05-cors-unanchored-regex.md](05-cors-unanchored-regex.md) | ✅ Đã sửa |
| 06 | Bypass rate-limit & chiếm session anonymous qua header client | 🟠 High | Security | [06-anonymous-ratelimit-session-bypass.md](06-anonymous-ratelimit-session-bypass.md) | ⬜ Chưa xử lý |
| 07 | `actx`: mutex khai báo nhưng không khoá → data race | 🟠 High | Shared | [07-actx-mutex-not-locked-datarace.md](07-actx-mutex-not-locked-datarace.md) | ✅ Đã sửa |
| 08 | Config nil-pointer panic khi thiếu section | 🟠 High | Config | [08-config-nil-pointer-panic.md](08-config-nil-pointer-panic.md) | ✅ Đã sửa |
| 09 | koanf nuốt lỗi đọc/parse → chạy config mặc định âm thầm | 🟠 High | Config | [09-koanf-swallow-config-errors.md](09-koanf-swallow-config-errors.md) | ✅ Đã sửa |
| 10 | Pub/Sub không reconnect → mất message âm thầm | 🟠 High | Pubsub | [10-pubsub-no-reconnect-message-loss.md](10-pubsub-no-reconnect-message-loss.md) | ⬜ Chưa xử lý |
| 11 | Trace ID không propagate qua middleware | 🟡 Medium | Observability | [11-trace-id-not-propagated.md](11-trace-id-not-propagated.md) | ⬜ Chưa xử lý |
| 12 | Token kết nối tái sử dụng + lộ qua URL | 🟡 Medium | Security | [12-conn-token-replay-and-url-leak.md](12-conn-token-replay-and-url-leak.md) | ⬜ Chưa xử lý |
| 13 | Heartbeat/Ack/ping bỏ qua rate limit | 🟡 Medium | Security | [13-heartbeat-ack-ping-bypass-ratelimit.md](13-heartbeat-ack-ping-bypass-ratelimit.md) | ⬜ Chưa xử lý |
| 14 | `CleanUp` luôn trả nil + luôn log Error | 🟡 Medium | Error handling | [14-cleanup-returns-nil-and-noisy-log.md](14-cleanup-returns-nil-and-noisy-log.md) | ⬜ Chưa xử lý |
| 15 | Cửa sổ mất message ở msg-hub `Consume` | 🟡 Medium | Messaging | [15-msghub-consume-message-loss-window.md](15-msghub-consume-message-loss-window.md) | ⬜ Chưa xử lý |
| 16 | `Decr/Incr` trả false-negative khi về đúng 0 | 🟡 Medium | Cache | [16-cache-decr-incr-false-negative-zero.md](16-cache-decr-incr-false-negative-zero.md) | ⬜ Chưa xử lý |
| 17 | Otel exporter nil khi disabled/thiếu section | 🟡 Medium | Otel | [17-otel-nil-exporter.md](17-otel-nil-exporter.md) | ⬜ Chưa xử lý |
| 18 | DynamoDB ghi đè `ConnectedAt`/`CreatedAt` khi reconnect | 🟡 Medium | Repository | [18-dynamodb-overwrite-connectedat-createdat.md](18-dynamodb-overwrite-connectedat-createdat.md) | ⬜ Chưa xử lý |
| 19 | `RecursiveBatchGetItem` có thể loop vô hạn | 🟡 Medium | Repository | [19-recursive-batch-get-infinite-loop.md](19-recursive-batch-get-infinite-loop.md) | ⬜ Chưa xử lý |
| 20 | Nhóm Low / chất lượng code | 🟢 Low | Nhiều | [20-low-priority-and-quality.md](20-low-priority-and-quality.md) | ⬜ Chưa xử lý |

## Đề xuất thứ tự xử lý
1. **#01** rate limiter deadlock (rủi ro outage, sửa vài dòng).
2. **#02, #03** netpoll read loop + worker pool (đúng đắn dữ liệu + shutdown).
3. **#04** DynamoDB conditional update (ghost record bẩn dữ liệu vĩnh viễn).
4. **#05, #06** CORS + anonymous bypass (bảo mật).
5. **#08, #09** config fail-fast rõ ràng lúc khởi động.

## Đã kiểm và loại (false positive)
- ❌ "Migration Postgres multi-statement sẽ fail": pgx v5 tự dùng **simple protocol khi `Exec` không tham số** → nhiều statement chạy được (đã xác minh trong source pgx v5.8.0). Migration OK.
- ✅ SQL injection: mọi query Postgres dùng `$N` placeholder — an toàn.
- ✅ DynamoDB pagination / batch-25 / batch-100 / delete parity / TTL đơn vị milli nhất quán — đều đúng.

## Chú giải trạng thái
- ⬜ Chưa xử lý &nbsp;·&nbsp; 🔧 Đang sửa &nbsp;·&nbsp; ✅ Đã sửa &nbsp;·&nbsp; ❌ Không đồng ý / bỏ qua &nbsp;·&nbsp; ⏭️ Để sau
