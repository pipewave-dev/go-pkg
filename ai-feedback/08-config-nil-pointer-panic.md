# 08 — Config nil-pointer panic khi thiếu section

- **Mức độ:** 🟠 High
- **Vùng:** Config / khởi động
- **Trạng thái:** ⬜ Chưa xử lý
- **File liên quan:**
  - [export/types/config.go](../export/types/config.go) (`EnvType`, `LoadDefault`, `Validate`)
  - [export/types/config_child.go](../export/types/config_child.go) (các `validate()`/`loadDefault()`)

## Mô tả

`EnvType` khai báo các block cấu hình là **con trỏ** (`*InfoT`, `*CorsT`, `*ActiveConnectionT`, `*PingCheckerT`, `*RateLimiterT`, `*WorkerPoolT`, `*OtelT`, `*ValkeyT`, `*DynamoDBT`, `*PostgresT`), và **không nơi nào** cấp phát `&T{}` cho chúng trước khi gọi `LoadDefault()`/`Validate()`.

Nếu YAML/env thiếu một block (rất thường, ví dụ không có key `CORS:`), `koanf.Unmarshal` để field đó `nil`. Sau đó `LoadDefault()`/`Validate()` gọi method như `(c *CorsT) validate()` → dereference receiver `nil` → **panic nil-pointer lúc khởi động**, với stack khó hiểu, thay vì thông báo validation rõ ràng như thiết kế "fail fast".

## Đề xuất sửa (chọn 1)

1. Cấp phát mọi sub-config về zero-value non-nil trước khi validate:
```go
func (e *EnvType) ensureNonNil() {
    if e.Info == nil { e.Info = &InfoT{} }
    if e.Cors == nil { e.Cors = &CorsT{} }
    // ... cho tất cả block
}
```
gọi `ensureNonNil()` đầu `LoadDefault()`.

2. Hoặc thêm guard nil trong từng `validate()`/`loadDefault()` (dài dòng hơn).

Khuyến nghị (1). Cân nhắc thêm: block bắt buộc (Valkey/DynamoDB/Postgres) mà thiếu thì báo lỗi rõ ràng thay vì im lặng dùng zero-value.

## Ghi chú review
> _(chỗ trống để bạn ghi quyết định)_
