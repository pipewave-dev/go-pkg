# 05 — CORS regex không neo (`^...$`) → nới lỏng origin allowlist

- **Mức độ:** 🟠 High
- **Vùng:** Security / middleware
- **Trạng thái:** ✅ Đã sửa
- **File liên quan:**
    - [core/delivery/module/2.1.mux.go](../core/delivery/module/2.1.mux.go) (`isAllowedOrigin`, `corsMiddleware`)
    - [config.yaml](../config.yaml), [example-config.yaml](../example-config.yaml) (`REGEX_ORIGINS`)

## Mô tả

```go
for _, pattern := range regexOrigins {
    matched, err := regexp.MatchString(pattern, origin)
    if err == nil && matched { return true }
}
```

`regexp.MatchString` khớp khi pattern xuất hiện ở **bất kỳ vị trí** trong chuỗi (không neo `^`/`$`). Ví dụ với pattern `https://.*\.example\.com` (như trong example-config), origin của attacker `https://x.example.com.attacker.com` vẫn **khớp** (chứa chuỗi con `https://x.example.com`) → middleware phản chiếu `Access-Control-Allow-Origin` cho origin attacker.

## Mức độ thực tế

- Hiện **không** set `Access-Control-Allow-Credentials: true` → cookie không gửi kèm cross-origin, và `/gw` upgrade WebSocket không đi qua CORS. Nên đây thiên về **nới lỏng allowlist / defense-in-depth bị suy yếu** hơn là chiếm quyền trực tiếp.
- Vẫn cần sửa: allowlist nên đúng như ý định cấu hình.

## Đề xuất sửa

1. Neo mọi regex: bọc thành `^(?:pattern)$` trước khi match.
2. Precompile một lần lúc load config bằng `regexp.MustCompile` (thay vì `MatchString` mỗi request × mỗi pattern — vừa an toàn vừa nhanh hơn).

```go
// lúc load config
compiled := make([]*regexp.Regexp, 0, len(regexOrigins))
for _, p := range regexOrigins {
    compiled = append(compiled, regexp.MustCompile("^(?:"+p+")$"))
}
// khi match
for _, re := range compiled {
    if re.MatchString(origin) { return true }
}
```
