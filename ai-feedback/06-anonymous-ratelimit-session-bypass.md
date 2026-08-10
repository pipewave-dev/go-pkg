# 06 — Bypass rate-limit & chiếm session anonymous qua header client `X-Pipewave-ID`

- **Mức độ:** 🟠 High
- **Vùng:** Security / rate-limit / connection identity
- **Trạng thái:** ✅ Đã fix (server-side); xem "Đã triển khai" bên dưới
- **File liên quan:**
    - [core/service/websocket/mediator/delivery/1.issue_tmp_token.go](../core/service/websocket/mediator/delivery/1.issue_tmp_token.go) (đọc `X-Pipewave-ID`)
    - [core/service/websocket/mediator/delivery/3.long_polling.go](../core/service/websocket/mediator/delivery/3.long_polling.go) (đọc `X-Pipewave-ID` trực tiếp, **không** qua `connTmpToken` — vector độc lập, xem mục bổ sung bên dưới)
    - [core/service/websocket/mediator/delivery/4.long_polling_send.go](../core/service/websocket/mediator/delivery/4.long_polling_send.go) (tương tự `3.long_polling.go`)
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

- Tôi rất muốn giữ `X-Pipewave-ID` vì một số lý do như là:
    - msgHub khi frontend mất kết nối tạm thời, reconnect vẫn nhận message (bởi vì anonymous nên message thường chỉ là quảng cáo)
- Websocket dành cho Anonymous user là tính năng hỗ trợ cho unauthenticated user, thường nhận quảng cáo, thông báo đại trà, có xu hướng nhận nhiều hơn gửi (nhận từ anonymous thường sẽ điền form - có captcha verify)
- Tính năng Websocket dành cho Anonymous như một cách support thêm, dùng một số case ít gặp

## Nhận xét & đề xuất bổ sung (2026-07-16)

Đã đọc lại code liên quan (`1.issue_tmp_token.go`, `rate_limiter.go`, `connection_mamanger.go`, `ws_auth.go`, `exchange-token.go`, `0.new.go`). Xác nhận finding chính xác:

- `WebsocketAuth` của anonymous chỉ có `InstanceID` (`UserID` rỗng — xem `AnonymousUserWebsocketAuthWithMetadata` trong `ws_auth.go`), và `InstanceID` = header `X-Pipewave-ID` client tự set, `IssueTmpToken` chỉ check non-empty chứ không xác thực gì thêm. Cả `anonymousLimiter` (rate_limiter.go) lẫn `anonymousConn` (connection_mamanger.go) đều key thẳng theo giá trị này → cả hai vector 6a/6b đều valid như mô tả.
- #12 (single-use `connTmpToken` qua `GetDel` trong `exchange-token.go`) đã fix nhưng **không giúp gì cho #06**: TTL 10s single-use chỉ bảo vệ token trao đổi, không bảo vệ giá trị `InstanceID` bên trong — attacker vẫn tự gọi `/issue-tmp-token` với `X-Pipewave-ID` tuỳ ý (kể cả trùng ID nạn nhân) để lấy connTmpToken hợp lệ riêng.

### Về lý do muốn giữ `X-Pipewave-ID` client-controlled

Lý do "msgHub cần resume sau khi mất kết nối tạm thời" **không mâu thuẫn** với việc server sinh ID — chỉ cần ID đó _bền qua reconnect_, không nhất thiết phải _client tự chọn giá trị_. Tách hai việc "định danh phiên" và "quyền chọn định danh":

1. **Server sinh `InstanceID`** (dùng `fn.NewNanoID()` như đang làm cho `connTmpToken`), trả về qua cookie riêng `HttpOnly` (vd `__pw_anon_iid`), **không** qua response body/header đọc được bằng JS.
    - Lần đầu kết nối: không có cookie hợp lệ → server sinh ID mới, set cookie (TTL dài hơn connTmpToken, tuỳ nhu cầu resume thực tế, vd vài giờ).
    - Lần sau (`/issue-tmp-token` gọi lại để reconnect): server đọc ID từ cookie đó thay vì tin header `X-Pipewave-ID` → msgHub vẫn resume đúng như thiết kế hiện tại.
    - Vì cookie `HttpOnly` + `SameSite=Strict` + random cao-entropy, attacker không đọc được (trừ khi có XSS) và không đoán được ID của nạn nhân → vector 6b (chiếm/đá session) coi như đóng, không cần đổi kiến trúc msgHub.
2. **Rate-limit theo IP là lớp bổ sung ở chính `/issue-tmp-token`, không thay thế rate-limit theo instance.** Kể cả khi ID đã server-issued, attacker vẫn có thể xoá cookie / gọi `/issue-tmp-token` liên tục để xin ID mới → cần giới hạn số lần issue-token/phút/IP ngay tại endpoint này. Việc này độc lập, không ảnh hưởng trải nghiệm resume của user thật.
3. Sau khi có #1 và #2, rate-limit cho traffic message (đang key theo `InstanceID`) vẫn giữ nguyên được — vì lúc này ID không còn free-mint được nữa, không cần đổi hẳn sang key theo IP thuần (tránh phạt oan nhiều user thật sau NAT chung/mobile carrier).

### Điểm cần cân nhắc thêm

- Đề xuất gốc "IP hoặc IP + ID do server cấp" cho rate-limiter — cá nhân nghiêng về **không** bind cứng theo IP cho connection/limiter chính, vì client di động roaming đổi IP giữa chừng sẽ bị mất session/limiter dù không phải attacker. IP chỉ nên dùng ở tầng issue-token để chặn việc mint ID hàng loạt.
- "Anonymous WS là tính năng phụ, ít gặp": đồng ý về ưu tiên xử lý (có thể lùi lịch), nhưng lưu ý 6a vẫn là unauthenticated flood/DoS primitive nhắm vào tài nguyên backend dùng chung (connection/goroutine/queue), ảnh hưởng vượt ra ngoài phạm vi tính năng anonymous — nên vẫn nên giữ mức High.

### Tóm tắt đề xuất ưu tiên

1. Server-issue `InstanceID` cho anonymous, lưu qua cookie `HttpOnly` (giữ được khả năng resume, đóng hoàn toàn vector 6b).
2. Thêm rate-limit theo IP tại `/issue-tmp-token` (đóng vector 6a kể cả khi ID đã server-issued).
3. Giữ nguyên rate-limiter theo `InstanceID` cho traffic sau khi đã kết nối, không cần đổi sang key IP thuần.

### Sửa Client package (typescript):

- client chính sử dụng project này code bằng typescript trong `
pipewave-js-sdk/packages`
- Client tự gửi header `X-Pipewave-ID` xuất hiện trong code `pipewave-js-sdk/packages/core/src/external/pipewave/clients/index.ts`
- Sau khi fix theo đề xuất của bạn, cần phải cân nhắc sử lại client hợp lý

## Phát hiện bổ sung (2026-07-16): `/lp` và `/lp-send` là vector độc lập, không đi qua `/issue-tmp-token`

Đọc `3.long_polling.go` và `4.long_polling_send.go`: cả hai endpoint build thẳng `wsAuth` từ `r.Header.Get("X-Pipewave-ID")` trên **mỗi request**, không hề gọi `exchangeToken.ScanConnToken` hay kiểm tra `connTmpToken` nào cả — khác hẳn `/gw` (`2.gobwas_endpoint.go`), vốn chỉ tin `connTmpToken` đã exchange trước đó. Nghĩa là chỉ vá `/issue-tmp-token` là không đủ: attacker có thể bỏ qua hoàn toàn bước issue-token và gọi thẳng `/lp` / `/lp-send` với `X-Pipewave-ID` tuỳ ý (miễn có JWT anonymous hợp lệ nào đó) để tái tạo y hệt 6a/6b.

## Quyết định cuối & đã triển khai (2026-07-16)

- Bổ sung nhận định: vector 6b không chỉ là "kick" session — `onNewStuff` (`0.new.go`) còn gọi `msgHubSvc.Consume(ctx, auth.UserID, auth.InstanceID)` để giao message đang buffer cho connection mới; với anonymous, key thực chất chỉ là `InstanceID` → attacker replay đúng ID còn **nhận được message đang chờ nạn nhân**, không chỉ đá session.

### Pivot: cookie `HttpOnly` → signed opaque token (quan trọng)

Thiết kế ban đầu (cookie `__pw_anon_iid`, `HttpOnly` + `SameSite=Strict`) **không hoạt động** với use-case thực tế chính của package `@pipewave/core` trong `pipewave-js-sdk`: đây là SDK publish npm, nhúng vào **frontend của khách hàng**, gọi tới domain API Pipewave — gần như chắc chắn khác origin với domain app khách hàng (`endpoint` là config do consumer truyền vào, xem `runtime.ts`). Với cross-site request, `SameSite=Strict` (và cả `Lax`) khiến browser **không gửi kèm cookie** — cơ chế server-issue coi như vô hiệu cho đúng nhóm khách hàng chính, ID lại quay về free-mint được qua đường vòng.

→ Đổi sang **signed opaque token**: server mint `nanoid:mintUnixTime`, ký HMAC-SHA256, trả về dạng `"<nanoid>:<ts>.<base64url(hmac)>"` — không phải cookie, chỉ là giá trị app-level client lưu và gửi lại qua `X-Pipewave-ID` như cũ. Không phụ thuộc cookie/SameSite nên hoạt động bình thường dù same-site hay cross-site. Vẫn giữ nguyên các tính chất bảo mật đã thiết kế: server-issued, không forge được (verify HMAC), chỉ mint tại `/issue-tmp-token` (bị IP-throttle), có TTL (6h, nhúng trong payload) giới hạn cửa sổ resume/rò rỉ.

- **Đã fix (server-side, Go):**
    1. `anon_instance.go` (mới): `anonymousInstanceSigner` — HMAC secret lấy từ config `ANONYMOUS_INSTANCE.SECRET` (bắt buộc, panic nếu rỗng; phải giống nhau và ổn định trên mọi replica). `mintOrReadAnonymousInstanceID` là điểm mint token duy nhất cho anonymous, chỉ `IssueTmpToken` gọi; `readAnonymousInstanceID` chỉ verify HMAC + hạn `anonymousInstanceMaxAge=6h`, không mint.
    2. `1.issue_tmp_token.go`: nhánh anonymous dùng `mintOrReadAnonymousInstanceID`; nhánh authenticated giữ nguyên (đã an toàn vì key theo `UserID` xác thực bằng JWT). Response đổi từ plain-text sang JSON `{connToken, instanceId?}` — `instanceId` chỉ có khi anonymous. Thêm IP rate-limit (`ip_rate_limiter.go`, config `RATE_LIMITER.ISSUE_TOKEN_IP_RATE/BURST`, mặc định 5/20) chặn việc mint token hàng loạt.
    3. `3.long_polling.go`, `4.long_polling_send.go`: nhánh anonymous đổi sang `readAnonymousInstanceID` (chỉ verify header, 400 nếu thiếu/sai chữ ký — bắt buộc phải gọi `/issue-tmp-token` trước, nơi duy nhất bị IP-throttle). Không tự mint để tránh mở lại vector 6a qua đường vòng.
    4. `rate_limiter.go` / `connection_mamanger.go`: giữ nguyên, key theo `InstanceID` (giờ là token đã ký) — an toàn vì không còn free-mint được.
    5. `export/types/config.go`, `config_child.go`, `config.yaml`: thêm `ANONYMOUS_INSTANCE.SECRET` (bắt buộc) và `RATE_LIMITER.ISSUE_TOKEN_IP_RATE/BURST`. **Lưu ý: đây là config bắt buộc mới** — service sẽ panic lúc khởi động nếu chưa set `ANONYMOUS_INSTANCE_SECRET` ở môi trường khác dev.
    6. Test: `anon_instance_test.go` (mint/reuse token hợp lệ, bỏ qua header không có chữ ký, từ chối token ký bằng secret khác, từ chối payload bị tamper, từ chối token hết hạn), `ip_rate_limiter_test.go`.
- **Đã fix (client TypeScript, `pipewave-js-sdk/packages/core`):**
    1. `clients/index.ts`: `RestClients.pipewaveIDPromise` giờ mutable qua `setPipewaveID()`; thêm `issueTmpToken()` dùng chung — gọi `/issue-tmp-token`, parse JSON, tự động `setPipewaveID(instanceId)` nếu server trả về (ghi đè ID client tự sinh ban đầu bằng token đã ký của server).
    2. `services/websocket/service.ts`: dùng `client.issueTmpToken()` thay vì fetch/parse thủ công.
    3. `services/long-polling/service.ts`: `startPollLoop()` gọi `client.issueTmpToken()` trước khi vào vòng poll — cần thiết vì trước đây LP có thể là transport đầu tiên (khi `sessionStorage` đã lưu `'lp'` từ phiên trước) và không hề gọi `/issue-tmp-token`; nay `/lp` bắt buộc phải có token đã mint trước.
