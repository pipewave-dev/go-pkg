# 02 — Vòng đọc netpoll edge-triggered: đọc thiếu + đọc đồng thời trên cùng conn

- **Mức độ:** 🟠 High
- **Vùng:** Concurrency / WebSocket transport
- **Trạng thái:** ⬜ Đã xử lý
- **File liên quan:**
  - [core/service/websocket/server/gobwas/1_server.go](../core/service/websocket/server/gobwas/1_server.go) (`NewConnection`, `handleClientData`, `processClientMessage`, `writeFrame`)

## Bối cảnh đã xác minh

`netpoll.HandleRead(conn)` = `Handle(conn, EventRead|EventEdgeTriggered)` → **EPOLLET (edge-triggered), KHÔNG oneshot** (xác minh trong source `github.com/mailru/easygo@.../netpoll/handle.go`). Với ET, epoll chỉ báo lại khi có **cạnh mới** (dữ liệu mới tới), không báo lại chừng nào socket vẫn còn dữ liệu chưa đọc hết.

Nhưng `processClientMessage` chỉ đọc **đúng một frame** mỗi lần callback (một `ReadHeader` + một `ReadFull`).

## Vấn đề

### 2a. Đọc thiếu (under-drain)
Nếu client gửi nhiều WebSocket frame về cùng lúc (gộp trong một TCP segment / một cạnh readiness), chỉ frame **đầu tiên** được đọc; phần còn lại kẹt trong kernel buffer. epoll **không báo lại** cho tới khi có dữ liệu mới → các frame sau bị **trễ/treo** đến lần client gửi kế tiếp.

### 2b. Đọc đồng thời trên cùng `net.Conn`
Mỗi cạnh readiness → `handleClientData` → `workerPool.Submit(processClientMessage)` (async). Vì xử lý tách khỏi vòng poll, một cạnh mới có thể tới **trước khi** worker của cạnh trước đọc xong → spawn worker thứ hai. Hai worker cùng gọi `ws.ReadHeader(conn)` / `io.ReadFull(conn, ...)` trên **cùng một `net.Conn`** → **data race + xen byte giữa hai frame → hỏng khung → protocol error/panic** trong thư viện `ws`.

> Ghi chú: đường **ghi** đã được bảo vệ bằng `client.writeMu` ([1_server.go:266-275](../core/service/websocket/server/gobwas/1_server.go#L266-L275)); đường **đọc** thì chưa có bảo vệ tương ứng.

## Đề xuất sửa (chọn 1)

1. **Drain toàn bộ mỗi event:** vòng lặp đọc frame trong `processClientMessage` cho tới khi `EAGAIN`/would-block, để đọc hết dữ liệu của một cạnh ET.
2. **Oneshot + Resume:** dùng `netpoll.HandleReadOnce` và gọi `poller.Resume(desc)` sau khi xử lý xong — vừa tuần tự hoá đọc, vừa tránh spawn worker trùng.
3. **CAS "đang xử lý":** cờ atomic per-connection để không submit/không chạy read-task mới khi còn một task đọc đang chạy cho cùng client (song hành với `writeMu`).

Khuyến nghị (2) hoặc (1)+(3) để giải quyết cả 2a lẫn 2b.

## Ghi chú review
> _(chỗ trống để bạn ghi quyết định)_

## Action:

- Đã sửa theo hướng **(2) Oneshot + Resume** trong `core/service/websocket/server/gobwas/1_server.go` và `1_type.go`:
  - `netpoll.HandleRead` (ET) → `netpoll.HandleReadOnce` (EPOLLONESHOT, level-triggered) khi tạo descriptor (`NewConnection`).
  - `handleClientData` sau khi `processClientMessage` xong thì gọi `resumeRead(client)` (mới thêm) để `poller.Resume(desc)` re-arm descriptor. Resume gọi từ worker goroutine (không phải trong callback của poller) để tránh deadlock theo doc của `netpoll.Poller.Start`.
  - Vì reArm là level-triggered, nếu còn dữ liệu chưa đọc hết (nhiều frame gộp — 2a), epoll báo lại ngay lập tức sau Resume → tiếp tục đọc frame kế tiếp cho tới khi hết dữ liệu, thay vì treo tới cạnh mới.
  - Vì oneshot chỉ báo đúng 1 lần cho tới khi được Resume, và chỉ Resume sau khi `processClientMessage` của lần đọc trước đã xong, nên không còn khả năng 2 worker cùng đọc trên 1 `net.Conn` (2b).
  - Thêm `descMu sync.Mutex` trên `GobwasConnection` để khoá chéo giữa `resumeRead()` (Resume) và `removeClient()` (Stop + `desc.Close()`), tránh race gọi `Resume` trên fd đã đóng (và có thể bị OS tái sử dụng cho kết nối khác).
  - Test hiện có (`go test -race ./core/service/websocket/server/gobwas/...`) pass.
