# 12 — Token kết nối tái sử dụng + lộ qua URL

- **Mức độ:** 🟡 Medium
- **Vùng:** Security / token
- **Trạng thái:** ⬜ Chưa xử lý
- **File liên quan:**
  - [core/service/websocket/mediator/delivery/2.gobwas_endpoint.go](../core/service/websocket/mediator/delivery/2.gobwas_endpoint.go) (đọc `?tk=`)
  - [core/service/websocket/exchange-token/exchange-token.go](../core/service/websocket/exchange-token/exchange-token.go) (`ScanConnToken`)
  - [core/service/websocket/exchange-token/0.0.new.go](../core/service/websocket/exchange-token/0.0.new.go) (`tokenTTL = 10`)
  - [pkg/mux-middleware/log_formater.go](../pkg/mux-middleware/log_formater.go) (log `RequestURI`)

## Mô tả

- **Không single-use:** `ScanConnToken` chỉ `Get` token từ cache, **không xoá** sau khi dùng. Trong cửa sổ TTL 10s, token có thể dùng lại nhiều lần (mở nhiều connection / replay).
- **Lộ qua URL:** token nằm ở query string `/gw?tk=...`. `JSONLogFmt` log `r.RequestURI` nguyên văn → token vào log; ngoài ra có thể lọt vào access log của LB/proxy/CDN, Referer, lịch sử.

TTL 10s đã giảm nhẹ, nhưng single-use là chuẩn.

## Đề xuất sửa

- **Single-use:** `ScanConnToken` xoá token ngay sau khi đọc (atomic get-and-delete nếu cache hỗ trợ) → thu hẹp cửa sổ replay về ~0.
- **Không log token:** redact tham số `tk` cho route `/gw` trong `JSONLogFmt` (hoặc bỏ query string khỏi log route này).
- (Tùy chọn) cân nhắc truyền token qua header `Sec-WebSocket-Protocol` thay vì query để tránh lộ URL.

Liên quan [#06](06-anonymous-ratelimit-session-bypass.md).

## Ghi chú review
> _(chỗ trống để bạn ghi quyết định)_
