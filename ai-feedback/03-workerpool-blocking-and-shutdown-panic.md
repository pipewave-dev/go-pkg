# 03 — WorkerPool: `Submit` blocking + panic khi shutdown

- **Mức độ:** 🟠 High
- **Vùng:** Concurrency / worker-pool
- **Trạng thái:** ⬜ Chưa xử lý
- **File liên quan:**
    - [pkg/worker-pool/1_worker_pool.go](../pkg/worker-pool/1_worker_pool.go) (`Submit`, `Close`)
    - [provider/worker-pool-provider/worker_pool.go](../provider/worker-pool-provider/worker_pool.go) (đăng ký shutdown priority)
    - [core/service/websocket/server/gobwas/1_server.go](../core/service/websocket/server/gobwas/1_server.go) (`handleClientData` → `Submit`)
    - [core/service/websocket/mediator/service/0.new.go](../core/service/websocket/mediator/service/0.new.go) (`Shutdown` priority Normal)

## Vấn đề 3a — `Submit` blocking gây nghẽn poller (backpressure DoS)

```go
func (p *WorkerPool) Submit(task func()) {
    p.taskQueue <- task   // blocking khi buffer đầy (config BUFFER=128)
}
```

`Submit` được gọi trực tiếp từ goroutine dispatch của netpoll (`handleClientData`). Khi `taskQueue` đầy (tải cao, hoặc handler chậm), goroutine poll **bị chặn** → không xử lý event cho **mọi connection khác** → đình trệ toàn cục.

**Sửa:** `Submit` non-blocking (select với `default` → trả lỗi/đếm drop + metric), hoặc tách vòng poll khỏi việc đẩy vào queue, hoặc tăng/độ co giãn worker theo threshold (đã có `UpperThreshold`/`LowerThreshold` nhưng chỉ gọi Action, không tự scale).

## Vấn đề 3b — Panic gửi vào channel đã đóng khi shutdown

`WorkerPool.Close()`:

```go
func (p *WorkerPool) Close() {
    close(p.done)
    close(p.taskQueue)   // đóng queue
    p.wg.Wait()
}
```

`Close()` đăng ký ở priority **Early** (`-100`), chạy **trước** `mediatorSvc.Shutdown()` (đóng connection) và `msgHubSvc.Shutdown()` ở priority **Normal** (`0`) — do fn-collector sort tăng dần. `NetpollServer` **không** dừng poller và **không** gate `healthy` cho connection đang sống, nên trong khoảng giữa 2 bước, một frame client tới → `handleClientData` → `Submit` → **gửi vào channel đã đóng → panic**. Panic này xảy ra trên goroutine dispatch của netpoll, **ngoài** `recover()` của worker → **crash cả process** lúc shutdown "graceful".

**Sửa (chọn 1):**

- Dừng/đóng poller (hoặc set `healthy=false` và cho `handleClientData` kiểm tra) **trước** khi `WorkerPool.Close()`.
- Hoặc chuyển `WorkerPool.Close()` xuống priority muộn hơn bước đóng connection.
- Hoặc `Submit` kiểm cờ atomic `closed` và trả lỗi thay vì gửi vào channel.

## Ghi chú review

- Chốt phương án: chuyển `WorkerPool.Close()` xuống priority muộn hơn bước đóng connection
    - Đảm bảo những nơi Submit task vào WorkerPool phải ngừng Submit, sau đó mới `WorkerPool.Close()`, worker pool sẽ chờ khi nào hết task
