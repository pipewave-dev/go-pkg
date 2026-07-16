# package `gobwas`

Đây là một implementation cụ thể của interface `wsSv.WebsocketServer`
(`core/service/websocket/1.server_type.go`), dùng hai thư viện:

- [`github.com/gobwas/ws`](https://github.com/gobwas/ws) — encode/decode frame RFC 6455 (không tự quản lý I/O loop, chỉ là codec).
- [`github.com/mailru/easygo/netpoll`](https://github.com/mailru/easygo) — wrapper Go quanh `epoll` (Linux) / `kqueue` (BSD) để biết **khi nào** một fd có dữ liệu để đọc, mà không cần một goroutine blocking-read cho mỗi connection.

Package chia làm 3 file:

| File | Nội dung |
|---|---|
| `1_type.go` | Định nghĩa struct `NetpollServer`, `GobwasConnection`, state máy ping/pong, `serverStats`. |
| `1_server.go` | Vòng đời connection: accept, đăng ký vào netpoll, đọc frame, ghi frame, đóng connection. |
| `2_frame_handle.go` | Validate + dispatch frame theo OpCode (text/binary/ping/pong/close/continuation). |
| `drain_test.go` | Test riêng cho cơ chế "drain" (giải thích ở cuối). |

---

## 1. `netpoll` là gì, và tại sao cần nó?

### Vấn đề cần giải quyết

Một WebSocket server "ngây thơ" thường có kiến trúc: mỗi connection = 1 goroutine, gọi
`conn.Read()` blocking trong vòng lặp `for`. Với vài nghìn connection, đó là vài nghìn
goroutine đang ngủ trong syscall `read()`, tốn ~2-8KB stack/goroutine + chi phí scheduler
phải quản lý ngần ấy goroutine.

`epoll` (Linux) là syscall cho phép **một thread duy nhất** hỏi kernel: "trong danh sách
hàng nghìn fd tôi đang theo dõi, fd nào đã có dữ liệu để đọc?" — thay vì bạn phải tự
blocking-read từng cái một. Đây chính là mô hình mà nginx, Redis, Node.js dùng để xử lý
hàng chục nghìn connection với rất ít thread.

`mailru/easygo/netpoll` là một lớp Go mỏng bọc quanh `epoll_create`/`epoll_ctl`/`epoll_wait`,
cho một API hướng callback thay vì phải tự viết syscall.

### Các khái niệm cốt lõi trong netpoll

**`netpoll.Poller`** (interface, `netpoll.go`)
```go
type Poller interface {
    Start(*Desc, CallbackFn) error
    Stop(*Desc) error
    Resume(*Desc) error
}
```
Đại diện cho 1 instance epoll. Trong package này chỉ có **một** `Poller` dùng chung cho
toàn bộ server (`NetpollServer.poller`), theo dõi fd của *mọi* connection.

**`netpoll.Desc`** (`handle.go`)
Một "descriptor" bọc quanh `os.File` (lấy được từ `net.Conn` qua `conn.File()`) + cấu hình
`Event` (đọc/ghi, edge/level-triggered, oneshot hay không). Đây là thứ bạn đưa cho
`Poller.Start/Stop/Resume`. Mỗi `GobwasConnection` giữ đúng 1 `Desc` (`cl.desc`).

Lưu ý ngầm quan trọng: `conn.File()` **dup** file descriptor gốc (dùng `dup()` syscall dưới
gầm net package), và as a side effect nó set fd gốc về **blocking mode**. Vì vậy
`netpoll.Handle()` phải tự gọi lại `setNonblock(fd, true)` để trả `net.Conn` (ví dụ
`SetReadDeadline`) về hoạt động đúng — đây là lý do có đoạn comment "Set the file back to
non blocking mode" trong `handle.go` của thư viện.

**`netpoll.Event`** — bitmask cấu hình cách theo dõi 1 fd:
- `EventRead` / `EventWrite` — theo dõi đọc hay ghi được.
- `EventEdgeTriggered` (map sang `EPOLLET`) — kernel chỉ báo **một lần** khi trạng thái
  chuyển từ "không sẵn sàng" → "sẵn sàng". Nếu bạn không đọc hết buffer trong lần đó, sẽ
  không có thông báo tiếp theo cho tới khi có dữ liệu **mới** đến.
- `EventOneShot` (map sang `EPOLLONESHOT`) — sau khi báo sự kiện đúng 1 lần, kernel **tự
  gỡ fd khỏi epoll set** (thực chất là set tạm event mask = 0 qua `EPOLL_CTL_MOD`). Muốn
  nhận sự kiện tiếp theo, bạn phải chủ động gọi `Poller.Resume(desc)` để "vũ trang lại"
  (re-arm).

  Về bản chất, `EPOLLONESHOT` vẫn là **level-triggered** (không phải edge): nếu khi bạn
  gọi `Resume()`, fd vẫn còn dữ liệu chưa đọc hết từ trước, epoll sẽ **báo lại ngay lập
  tức** — khác với edge-triggered là chỉ báo khi có dữ liệu *mới*.

**`netpoll.HandleReadOnce(conn)`** — helper tương đương
`Handle(conn, EventRead|EventOneShot)`. Đây là hàm được dùng trong `1_server.go:125`.

### Package này dùng netpoll như thế nào

Đoạn code + comment quan trọng nhất nằm ở `1_server.go:117-153`:

```go
desc, err := netpoll.HandleReadOnce(client.conn)
...
client.desc = desc

err = s.poller.Start(desc, func(ev netpoll.Event) {
    if ev&netpoll.EventReadHup != 0 {
        client.Close()
        return
    }
    s.handleClientData(client)
})
```

Vì sao chọn `HandleReadOnce` (oneshot, level-triggered) thay vì `HandleRead`
(edge-triggered, không oneshot)?

1. **An toàn concurrency theo thiết kế, không cần lock riêng cho việc đọc.**
   Với oneshot, ngay khi epoll báo sự kiện, kernel tự gỡ fd khỏi danh sách theo dõi. Nghĩa
   là callback tiếp theo **chắc chắn không xảy ra** cho tới khi bạn gọi `Resume()`. Do đó
   tại một thời điểm, **tối đa một** `processClientMessage` đang chạy cho một connection —
   không có 2 goroutine cùng đọc trên 1 `net.Conn`, dù `ws.ReadHeader`/`io.ReadFull` tự nó
   không thread-safe.

2. **Không rơi vào bug kinh điển của edge-triggered.** Nếu dùng edge-triggered
   (`HandleRead`) mà không đọc hết buffer trong 1 lần callback, dữ liệu còn sót lại sẽ
   "im lặng" cho tới khi có gói tin *mới* tới — có thể gây stall vô thời hạn nếu client im
   lặng sau đó. Level-triggered (kể cả oneshot) không có vấn đề này: nếu bạn resume mà dữ
   liệu vẫn còn, epoll báo lại ngay.

3. **Không phải tạo goroutine đọc riêng cho từng connection** — callback của
   `Poller.Start` chỉ được kernel gọi *khi thực sự có dữ liệu*, nên việc dispatch vào
   worker pool (`handleClientData`) chỉ tốn tài nguyên khi có việc thật để làm.

### Vòng đời một lần đọc dữ liệu (read cycle)

```
epoll_wait() (trong netpoll's Epoll.wait, chạy nền)
        │  fd sẵn sàng đọc
        ▼
callback trong Poller.Start (chạy trên goroutine wait loop của netpoll!)
        │
        ├─ EventReadHup? → client.Close()  (client đóng nửa-đọc / RST)
        │
        ▼
s.handleClientData(client)
        │  KHÔNG chạy đọc trực tiếp ở đây — chỉ Submit task vào workerPool
        ▼
s.workerPool.Submit(func() {
    s.processClientMessage(client)   // đọc header+payload, validate, dispatch
    s.resumeRead(client)             // re-arm oneshot desc
})
```

Có 2 điều tinh tế đáng chú ý:

**(a) Vì sao không đọc ngay trong callback của `poller.Start`?**
Callback chạy trên goroutine nền độc quyền của `netpoll.Epoll.wait()` — cái vòng lặp gọi
`epoll_wait` cho *toàn bộ* server. Nếu bạn block ở đó (đọc socket, xử lý business logic),
bạn chặn luôn việc phát hiện sự kiện cho **mọi connection khác**. Đó là lý do
`handleClientData` chỉ `Submit` một closure vào `workerPool` rồi trả về ngay — việc đọc
thật sự (`processClientMessage`) diễn ra trên goroutine của worker pool.

**(b) Vì sao `resumeRead` được gọi *sau khi* đọc xong, từ worker goroutine, chứ không phải
ngay trong callback?**
Comment ở `1_server.go:168-174` giải thích: gọi `Resume()` ngay bên trong callback của
chính `Poller.Start` có thể **deadlock** — đây là ràng buộc ghi rõ trong doc của
`netpoll.Poller.Start`: *"Resume() call directly inside desc's callback could cause
deadlock"*. Nguyên nhân: `Resume()` (→ `epoll_ctl(MOD)`) và cơ chế wait loop chia sẻ một
mutex nội bộ trong `Epoll`; gọi lồng vào nhau trong cùng callback có thể tự khóa chính nó.
Giải pháp: tách `Resume()` ra khỏi callback, đưa vào goroutine của worker — chạy **sau
khi** `processClientMessage` xử lý xong.

Hệ quả của toàn bộ thiết kế: **đúng một worker tại một thời điểm được phép đọc từ một
connection**, và nó chỉ được phép đọc lại sau khi lần đọc trước đã xử lý xong và
re-arm — tạo ra một kiểu "hàng đợi tự nhiên" từng-frame-một cho mỗi connection, mà không
cần mutex tường minh nào bảo vệ việc đọc.

### `descMu` — cuộc đua giữa `resumeRead` và `removeClient`

```go
type GobwasConnection struct {
    ...
    desc   *netpoll.Desc
    descMu sync.Mutex
    ...
}
```

Có 2 nơi đụng vào `desc`:
- `resumeRead()` — gọi `poller.Resume(desc)` sau mỗi lần đọc xong.
- `removeClient()` — gọi `poller.Stop(desc)` + `desc.Close()` khi connection đóng (dù do
  lỗi, do nhận frame Close, hay do `EventReadHup`).

Nếu không có `descMu`, có race: `resumeRead` có thể gọi `Resume()` trên một fd mà
`removeClient` đã `Close()` — và tệ hơn, **hệ điều hành có thể tái sử dụng số fd đó cho
một connection hoàn toàn khác** ngay sau khi close (fd chỉ là một số nguyên nhỏ được cấp
phát lại). `Resume()` trên fd "ma" đó sẽ vô tình bật lại polling cho connection của người
khác. `descMu` + việc `removeClient` set `client.desc = nil` trước khi `Close()` (dưới
cùng 1 lock) đảm bảo `resumeRead` luôn thấy nhất quán: hoặc desc còn sống và thao tác hợp
lệ, hoặc đã bị dọn và no-op.

### So sánh nhanh: netpoll model vs "goroutine-per-connection" truyền thống

| | goroutine-per-conn (blocking read) | netpoll + worker pool (package này) |
|---|---|---|
| Số goroutine khi rảnh | 1 goroutine/connection, đang block trong syscall | 0 — chỉ 1 goroutine wait loop dùng chung cho cả server |
| Bộ nhớ / connection | Stack goroutine (~2-8KB, có thể grow) | Chỉ 1 `*Desc` + struct connection nhỏ (comment `PrintStats` trong code có ước tính "vs ~8KB in standard") |
| CPU khi có N connection im lặng | Scheduler vẫn phải theo dõi N goroutine (dù blocked) | `epoll_wait` không tốn CPU cho fd không có event |
| Độ phức tạp code | Đơn giản, tuyến tính | Phức tạp hơn: cần lo về oneshot/resume, race trên desc, không đọc block trong callback |

Đây chính xác là đánh đổi cổ điển "C10K problem" — package này chọn mô hình
event-driven để phục vụ rất nhiều connection với ít tài nguyên hệ thống.

### Bảng trade-off chi tiết

| Tiêu chí | 1 goroutine / 1 connection | netpoll + worker pool |
|---|---|---|
| **Mô hình xử lý** | Mỗi connection có 1 goroutine riêng, gọi `conn.Read()` blocking trong vòng lặp `for`. Song song 1:1 giữa connection và goroutine. | 1 event loop (`epoll_wait`) dùng chung cho **mọi** connection, chỉ đánh thức khi có dữ liệu thật; việc đọc/xử lý thật sự được giao cho N goroutine cố định trong `workerPool`. |
| **Số lượng goroutine tối đa** | Tăng tuyến tính theo số connection (10k connection ≈ 10k goroutine). | Cố định theo cấu hình `workerPool.Workers`, không phụ thuộc số connection. |
| **Bộ nhớ ở quy mô lớn** | Goroutine Go có stack khởi tạo ~2-8KB, có thể grow theo nhu cầu — 10k connection idle vẫn tốn hàng chục MB chỉ riêng cho stack. | Mỗi connection chỉ là 1 `*netpoll.Desc` (1 `os.File` + ít field) + struct `GobwasConnection` nhỏ — rẻ hơn nhiều lần trên mỗi connection khi số lượng lớn. |
| **Áp lực lên Go scheduler (GMP)** | Runtime phải theo dõi và scheduling hàng chục nghìn goroutine (dù đa số đang blocked chờ I/O) — tăng chi phí context-switch và GC quét stack. | Runtime chỉ quản lý goroutine wait loop + N worker cố định — độ phức tạp không phình theo số connection. |
| **Độ trễ xử lý 1 request** | Thấp và ổn định: goroutine đã "đứng sẵn" chờ đúng connection đó, dữ liệu tới là xử lý ngay. | Có thêm 1 bước gián tiếp: epoll báo sự kiện → `Submit` vào channel `taskQueue` → chờ tới lượt 1 worker rảnh. Nếu pool đang bận (queue đầy), latency tăng theo hàng đợi. |
| **Công bằng giữa các connection (fairness)** | Tự nhiên công bằng — mỗi connection có tài nguyên (goroutine) riêng, connection này không ảnh hưởng connection khác. | Nếu 1 worker bị một `onTextMessage`/`onBinMessage` chạy lâu (business logic chậm) chiếm giữ, nó không ảnh hưởng gián tiếp tới các connection khác *trừ khi* toàn bộ pool bị bão hòa — lúc đó mọi connection cùng bị trễ (hiệu ứng "noisy neighbor" ở mức queue dùng chung). |
| **Backpressure / quá tải** | Không có cơ chế tự nhiên — số goroutine cứ tăng theo connection mới, dễ tới mức runtime "sập" vì quá nhiều goroutine trước khi kịp phản ứng. | Có tín hiệu backpressure rõ ràng: `workerPool` tự theo dõi độ dài `taskQueue` mỗi 500ms, bắn `UpperThreshold.Action`/`LowerThreshold.Action` (dùng để cảnh báo/điều chỉnh healthy-check) khi hàng đợi vượt ngưỡng. |
| **Rủi ro race condition / độ phức tạp đồng bộ hoá** | Thấp — mỗi connection tự nhiên tuần tự trong 1 goroutine, ít cần lock ngoài việc ghi (nếu có nhiều writer). | Cao hơn hẳn: cần `descMu` để tránh race giữa `resumeRead`/`removeClient` trên cùng fd (kể cả nguy cơ OS tái sử dụng fd), cần đảm bảo không gọi `Resume()` lồng trong callback netpoll (deadlock), cần CAS (`closed`, `closeTx`, `closeRx`) để các đường đóng connection song song không dẫm lên nhau. |
| **Khả năng mở rộng (scale) theo số connection** | Giới hạn thực tế bởi bộ nhớ + chi phí scheduler khi số connection lên tới hàng chục-hàng trăm nghìn (bài toán C10K/C100K kinh điển). | Thiết kế đúng cho chính bài toán C10K/C100K — đây là lý do các server hiệu năng cao (nginx, Redis, Node.js libuv) đều dùng mô hình event loop + epoll/kqueue tương tự. |
| **Độ khó debug / đọc code** | Stack trace của goroutine gắn liền với 1 connection cụ thể — dễ trace logic của "1 connection" từ đầu đến cuối bằng 1 goroutine dump. | Logic của "1 connection" bị chẻ nhỏ qua nhiều lớp gián tiếp (callback netpoll → task trong worker pool) — khó hình dung tuyến tính hơn, cần hiểu rõ toàn bộ cơ chế oneshot/resume mới nắm được vòng đời đọc. |
| **Phù hợp khi nào** | Số connection vừa phải (vài trăm–vài nghìn), ưu tiên code đơn giản, dễ maintain, ít lo về race condition tinh vi. | Số connection lớn (chục nghìn+) hoặc muốn tối ưu tài nguyên hệ thống tối đa, chấp nhận độ phức tạp code cao hơn để đổi lấy khả năng chịu tải. |

Tóm lại: package này đánh đổi **độ phức tạp/khó đọc của code** để lấy **khả năng chịu tải
và hiệu quả tài nguyên** ở quy mô lớn — hợp lý cho một WebSocket gateway phục vụ số lượng
lớn client đồng thời, nhưng là "over-engineering" nếu số connection thực tế chỉ vài trăm.

---

## 2. `NetpollServer` — struct trung tâm (`1_type.go`)

```go
type NetpollServer struct {
    c           configprovider.ConfigStore
    poller      netpoll.Poller     // 1 epoll instance dùng chung
    healthy     healthyprovider.Healthy
    connections atomic.Int64       // đếm connection đang mở
    stats       *serverStats
    workerPool  *workerpool.WorkerPool

    onTextMessage wsSv.OnTextMessageFn
    onBinMessage  wsSv.OnBinMessageFn
    onReadError   wsSv.OnReadErrorFn
    onWriteError  wsSv.OnWriteErrorFn
    onClose       wsSv.OnCloseStuffFn
}
```

- **`workerPool`** (`pkg/worker-pool`): một pool goroutine cố định (`Workers` con số cấu
  hình) đọc task từ 1 channel (`taskQueue`) có buffer. Mọi việc "thật sự tốn CPU" (đọc
  frame, gọi callback business logic) đều đi qua `Submit(func(){...})` để không bao giờ
  chạy trên goroutine nhạy cảm của netpoll. Pool còn tự theo dõi độ dài queue mỗi 500ms để
  bắn `UpperThreshold.Action`/`LowerThreshold.Action` (dùng làm cảnh báo backpressure, ví
  dụ để tắt healthy-check khi quá tải).
- **`healthy`**: cờ global cho biết server có đang nhận connection mới không (dùng khi
  shutting down).
- **`onTextMessage` / `onBinMessage` / `onReadError` / `onWriteError` / `onClose`**: các
  callback được người dùng package (business layer) truyền vào lúc `NewServer(...)`, kiểu
  dependency injection — package `gobwas` chỉ lo phần transport, không biết gì về logic
  nghiệp vụ.
- **`NewServer` dùng `sync.Once`** → server này là **singleton** trong process (biến
  package-level `server`, `once`). Gọi `NewServer` nhiều lần chỉ tạo 1 lần, các lần sau
  trả về cùng instance, bỏ qua tham số truyền vào lần sau.

### `GobwasConnection` — 1 connection

```go
type GobwasConnection struct {
    c      configprovider.ConfigStore
    conn   net.Conn
    server *NetpollServer
    auth   voAuth.WebsocketAuth
    desc   *netpoll.Desc
    descMu sync.Mutex

    closed  atomic.Int32   // 0/1, đã bị removeClient() chưa
    closeTx atomic.Int32   // 0/1, đã gửi close frame chưa (CAS-guard)
    closeRx atomic.Int32   // 0/1, đã nhận close frame chưa

    writeMu sync.Mutex     // 1 writer tại 1 thời điểm lên conn
    stateMu sync.Mutex     // bảo vệ lastReadAt/lastPingAt/lastPongAt/awaitingPong

    lastReadAt   time.Time
    lastPingAt   time.Time
    lastPongAt   time.Time
    awaitingPong bool

    drainMu sync.RWMutex   // xem phần "drain" bên dưới
}
```

Nó implement 2 interface từ package `websocket` cha:
```go
var (
    _ wsSv.WebsocketServer = (*NetpollServer)(nil)
    _ wsSv.WebsocketConn   = (*GobwasConnection)(nil)
    _ wsSv.DrainableConn   = (*GobwasConnection)(nil)
)
```
— compile-time assertion quen thuộc trong Go để đảm bảo struct thỏa interface, dù không
có biến nào thật sự dùng các con trỏ nil đó.

### Ping/Pong keepalive — state machine trong `nextPingAction`

```go
func (cl *GobwasConnection) nextPingAction() pingAction {
    pingIdleAfter := cl.c.Env().PingChecker.PingIdleAfter
    pongTimeout := cl.c.Env().PingChecker.PongTimeout
    now := time.Now()
    ...
    if cl.awaitingPong {
        if now.Sub(cl.lastPingAt) >= pongTimeout {
            return pingActionClose   // client không phản hồi pong đúng hạn → coi như chết
        }
        return pingActionSkip        // đang chờ pong, chưa hết hạn → bỏ qua lần check này
    }
    if now.Sub(cl.lastReadAt) < pingIdleAfter {
        return pingActionSkip        // vẫn còn "mới" (có đọc được gì đó gần đây) → chưa cần ping
    }
    cl.awaitingPong = true
    cl.lastPingAt = now
    return pingActionSend            // im lặng đủ lâu → gửi ping, chờ pong
}
```

Đây là kiểu keepalive chuẩn cho WebSocket: server chủ động ping khi không thấy hoạt động
gì (kể cả text/binary frame — `noteRead` cũng reset `lastReadAt`), và nếu không có pong
trong `PongTimeout`, coi connection là chết và đóng. Việc gọi hàm này định kỳ là trách
nhiệm của lớp cao hơn (`WsService.PingAllLocalConnections()` trong
`core/service/websocket/0.ws_service.go`) — package `gobwas` chỉ cung cấp
`(*GobwasConnection).Ping()` làm entrypoint, còn *ai gọi nó và theo chu kỳ nào* nằm ngoài
package này.

---

## 3. Vòng đời connection trong `1_server.go`

### `NewConnection` — nhận 1 connection mới

1. Kiểm tra `s.healthy.IsHealthy()` — nếu server đang shutdown, từ chối luôn.
2. Validate `conn` không nil, và nếu là `*net.TCPConn`, thử `tcpConn.File()` để chắc chắn
   lấy được fd hợp lệ (rồi đóng `file` đó ngay — chỉ dùng để "test" khả năng lấy fd, vì
   `netpoll.Handle` bên trong sẽ tự gọi `File()` lại một lần "thật" sau).
3. Tăng counter (`stats.ConnectionsAccepted`, `s.connections`).
4. Tạo `GobwasConnection`, gọi `netpoll.HandleReadOnce(conn)` để lấy `Desc`.
5. `s.poller.Start(desc, callback)` — bắt đầu theo dõi fd này. Từ đây, mọi lần
   client gửi dữ liệu (hoặc đóng kết nối) sẽ trigger callback đã mô tả ở mục 1.

Nếu bất kỳ bước nào lỗi, connection được dọn dẹp (`conn.Close()`, giảm counter) và trả
`aerror.AError` — package dùng kiểu lỗi riêng `aerror` (có vẻ là "application error" chuẩn
hóa toàn hệ thống pipewave, đi kèm `actx`/trace ID) thay vì `error` trần.

### `processClientMessage` — đọc 1 frame

```go
header, err := ws.ReadHeader(conn)   // đọc 2-14 byte header WS frame
...
if header.Length > MaxFrameSize {    // giới hạn 1MB / frame — chống frame khổng lồ (DoS nhẹ)
    handleProtocolError(...)
    return
}
payload := make([]byte, header.Length)
io.ReadFull(conn, payload)           // đọc đúng header.Length byte
frame := ws.Frame{Header: header, Payload: payload}
validateFrame(frame)                  // RSV bits, opcode hợp lệ, control frame không fragment
client.noteRead(time.Now())
s.handleFrame(client, frame)
```

`ws.ReadHeader`/`ws.WriteFrame` đến từ `gobwas/ws` — thư viện đó **chỉ làm codec** (parse
bytes ⇄ struct `ws.Frame`/`ws.Header`), nó không tự mở goroutine hay quản lý I/O
scheduling — đó là lý do cần `netpoll` ở tầng trên để biết *khi nào* nên gọi các hàm này.

Một điểm cần lưu ý: `io.ReadFull` ở đây **có thể block** nếu payload chưa tới hết (TCP có
thể tới theo nhiều segment). Vì thiết kế oneshot + worker pool, việc block ở đây chỉ chiếm
1 worker goroutine (không phải goroutine của netpoll wait loop), nên chấp nhận được — đây
chính là lý do phải tách "đọc" ra khỏi callback netpoll như đã giải thích ở trên.

### Ghi dữ liệu: `send` / `writeFrame` / `writeMu`

```go
func (s *NetpollServer) writeFrame(client *GobwasConnection, frame ws.Frame) error {
    client.writeMu.Lock()
    defer client.writeMu.Unlock()
    if client.conn == nil {
        return net.ErrClosed
    }
    return ws.WriteFrame(client.conn, frame)
}
```
`writeMu` đảm bảo tại một thời điểm chỉ có 1 goroutine ghi lên cùng 1 `net.Conn` — cần
thiết vì `send()` (do business logic gọi qua `Send`/`SendDirect`) và `ping()`/
`handleProtocolError`/`handlePingFrame` (do chính server gọi khi xử lý frame) đều có thể
ghi đồng thời từ các worker khác nhau.

### `removeClient` — dọn dẹp, idempotent

```go
func (s *NetpollServer) removeClient(client *GobwasConnection) {
    if !client.closed.CompareAndSwap(0, 1) {
        return   // đã bị đóng bởi luồng khác rồi — no-op
    }
    ...
}
```
`CompareAndSwap(0, 1)` trên `atomic.Int32` là "chốt" quan trọng: nhiều nơi có thể gọi
`Close()`/`removeClient` gần như đồng thời (ví dụ: nhận `OpClose` frame *và* cùng lúc
`EventReadHup` bắn lên, hoặc lỗi ghi + lỗi đọc cùng lúc) — nhờ CAS, phần dọn dẹp thật sự
(`poller.Stop`, `desc.Close()`, `conn.Close()`, `onClose.Do(...)`) chỉ chạy đúng 1 lần.

---

## 4. Xử lý frame theo OpCode (`2_frame_handle.go`)

`validateFrame` kiểm tra 2 điều theo RFC 6455 trước khi dispatch:
- **RSV bits phải = 0** (server này không đàm phán extension nào, ví dụ `permessage-deflate`).
- **Control frame (`Close`/`Ping`/`Pong`) không được fragment** — luôn phải có `FIN=1`.

`handleFrame` unmask payload nếu cần (client → server luôn phải mask theo spec) rồi
dispatch theo opcode:

| OpCode | Xử lý |
|---|---|
| `OpText` / `OpBinary` | Gọi `onTextMessage`/`onBinMessage` (callback injected từ ngoài), kèm 1 `sendFn` closure để business logic có thể reply ngay trong callback. **Lưu ý**: code hiện tại xử lý fragment (`fin=false`) y hệt như frame hoàn chỉnh (comment "For now, treat as complete message" / "Future: Implement message fragmentation") — tức là **chưa** thật sự support message fragmentation, giả định client (SDK riêng của Pipewave) không gửi frame chia mảnh. |
| `OpContinuation` | Chỉ log, không xử lý gì — vì lý do trên. |
| `OpClose` | `MarkCloseReceived()`, parse close code (2 byte đầu, big-endian) + reason, phản hồi lại đúng 1 lần bằng `writeCloseOnce`, rồi `client.Close()`. Đúng chuẩn RFC 6455 "closing handshake". |
| `OpPing` | Trả lời `Pong` với cùng payload — bắt buộc theo spec. |
| `OpPong` | Gọi `notePong()` — xác nhận round-trip ping/pong server-initiated còn sống. |

`handleProtocolError`: khi có vi phạm giao thức (RSV bits sai, opcode lạ, frame quá lớn,
control frame bị fragment...), gửi 1 close frame với `ws.StatusProtocolError` rồi đóng
connection — không cố gắng phục hồi.

`writeCloseOnce` dùng `client.MarkCloseSentIfFirst()` (CAS trên `closeTx`) để đảm bảo
**chỉ gửi đúng 1 close frame**, dù `handleCloseFrame` và `handleProtocolError` (hoặc cả
2 do 2 lỗi xảy ra gần như đồng thời) đều có thể gọi tới.

---

## 5. Cơ chế "Drain" (`drainMu`, `BeginDrain`/`EndDrain`/`SendDirect`)

```go
func (cl *GobwasConnection) Send(ctx context.Context, payload []byte) error {
    cl.drainMu.RLock()
    defer cl.drainMu.RUnlock()
    ...
}
func (cl *GobwasConnection) BeginDrain() { cl.drainMu.Lock() }   // write lock
func (cl *GobwasConnection) EndDrain()   { cl.drainMu.Unlock() }
func (cl *GobwasConnection) SendDirect(ctx context.Context, payload []byte) error {
    // Không lock — caller PHẢI đang giữ write lock (giữa BeginDrain/EndDrain)
    ...
}
```

Đây là interface `wsSv.DrainableConn` (định nghĩa & doc chi tiết ở
`core/service/websocket/2.connection_type.go`). Use case: khi 1 connection tạm thời mất
kết nối rồi reconnect (session resume), có thể có **hàng đợi tin nhắn tồn đọng** cần gửi
lại **đúng thứ tự**, *trước* bất kỳ tin nhắn mới nào đang được các goroutine khác cố gắng
`Send()` cùng lúc.

Cơ chế dùng `sync.RWMutex` một cách "đảo ngược" khéo léo:
- **`Send()` bình thường** lấy **read lock** — nhiều `Send()` chạy song song được (đúng
  chất RWMutex), nhưng tất cả **bị chặn** nếu có ai đó đang giữ **write lock**.
- **Khi cần drain** (gửi hàng đợi tồn đọng): gọi `BeginDrain()` (write lock) — điều này
  **chặn mọi `Send()` mới** cho tới `EndDrain()`. Trong lúc giữ write lock, code gọi
  `SendDirect()` liên tiếp (không cần lock lại vì đã có write lock rồi — RWMutex không
  reentrant nên `SendDirect` phải né việc lock lần nữa) để đẩy các tin nhắn tồn đọng theo
  đúng thứ tự. Sau `EndDrain()`, các `Send()` đang chờ mới được tiếp tục — và vì hàng đợi
  đã được đẩy đi hết trước đó, thứ tự tin nhắn "cũ trước, mới sau" được đảm bảo.

`drain_test.go` verify chính xác thuộc tính này: 5 goroutine gọi `send("new")` đồng thời
trong lúc `drainMu` bị khóa, rồi drain 2 message `"pending-1"`, `"pending-2"` bằng
`sendDirect`, rồi mở khóa — test khẳng định `pending-1`/`pending-2` luôn nằm ở đầu danh
sách kết quả.

---

## 6. Tổng kết luồng dữ liệu end-to-end

```
                        ┌─────────────────────────────────────┐
                        │   netpoll.Epoll.wait() (1 goroutine) │
                        │   epoll_wait() cho TẤT CẢ connection │
                        └───────────────┬───────────────────────┘
                                        │ fd sẵn sàng đọc
                                        ▼
                     poller.Start callback (1_server.go:144)
                                        │
                              EventReadHup? → client.Close()
                                        │ else
                                        ▼
                     s.handleClientData(client) — chỉ Submit(), không đọc
                                        │
                                        ▼
                     ╔═══════════ workerpool goroutine ═══════════╗
                     ║ s.processClientMessage(client)             ║
                     ║   ws.ReadHeader → io.ReadFull → validate   ║
                     ║   s.handleFrame → dispatch theo OpCode     ║
                     ║     OpText/OpBinary → onTextMessage/       ║
                     ║       onBinMessage (business logic)        ║
                     ║     OpPing → writeFrame(Pong)              ║
                     ║     OpClose → writeCloseOnce + Close()     ║
                     ║ s.resumeRead(client) → poller.Resume(desc) ║
                     ╚═════════════════════════════════════════════╝
                                        │
                          (đợi lần epoll_wait tiếp theo báo có dữ liệu)
```

Bản chất package này là một **event loop dùng chung** (netpoll) cộng với một
**worker pool để không block event loop đó**, thay vì mô hình 1 goroutine/1 connection
blocking-read truyền thống — đánh đổi độ phức tạp code để lấy khả năng chịu tải nhiều
connection đồng thời với ít tài nguyên hệ thống hơn.
