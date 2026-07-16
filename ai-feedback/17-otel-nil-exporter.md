# 17 — Otel exporter nil khi disabled/thiếu section

- **Mức độ:** 🟡 Medium
- **Vùng:** Otel / observability
- **Trạng thái:** ✅ Đã xử lý
- **File liên quan:**
    - [provider/otel-provider/otel.go](../provider/otel-provider/otel.go) (`NewDI`, dòng 15-40)
    - [pkg/otel/connect.go](../pkg/otel/connect.go) (`newTraceProvider`, switch exporter 76-131)

## Mô tả

- `otelprovider.NewDI` dựng `OtelConfig` và gọi `NewOtelProvider` **vô điều kiện** — **không** kiểm `env.Otel.Enabled`.
- `OtelT` có `validate()` (chỉ enforce khi `Enabled`) nhưng **không** `loadDefault()`, và `EnvType.LoadDefault()` không gọi cho `Otel`. Nên khi otel tắt (hoặc section vắng — xem [#08](08-config-nil-pointer-panic.md)), `ExporterType == ""`.
- Trong `newTraceProvider`, `switch s.config.ExporterType` **không có `default`** → `traceExporter` giữ `nil` → `trace.NewTracerProvider(..., trace.WithBatcher(nil, ...))` → tracer provider hỏng (spans đẩy vào exporter nil).

→ Ngược với kỳ vọng "tracing tắt": thay vì no-op, lại dựng provider hỏng.

## Đề xuất sửa

- Kiểm `env.Otel.Enabled` trong `NewDI`: nếu tắt → trả no-op tracer provider.
- Thêm `default:` trong switch exporter: fallback `"discard"` hoặc trả lỗi rõ ràng.
- Bổ sung `loadDefault()` cho `OtelT` (mặc định `ExporterType: "discard"`).
