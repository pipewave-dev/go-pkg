# 09 — koanf nuốt lỗi đọc/parse → chạy config mặc định âm thầm

- **Mức độ:** 🟠 High
- **Vùng:** Config / koanf
- **Trạng thái:** ✅ Đã sửa
- **File liên quan:**
    - [pkg/koanf/0_interactor.go](../pkg/koanf/0_interactor.go) (dòng ~101 `os.ReadFile`, ~116 `Load`)
    - [pkg/koanf/unmarshall.go](../pkg/koanf/unmarshall.go) (`Unmarshall`)

## Mô tả

```go
raw, _ := os.ReadFile(abspath)                 // lỗi đọc bị bỏ
...
k.koanf.Load(env.Provider(...), nil)           // lỗi Load bị bỏ
...
func (k *koanfProvider) Unmarshall(output any) {
    _ = k.koanf.Unmarshal("", output)          // lỗi decode bị bỏ
}
```

Theo doc comment của `FromYaml`: "tất cả file là bắt buộc" / "panic nếu không load được". Nhưng thực tế:

- File bắt buộc **không tồn tại** → `os.ReadFile` trả `nil, err`, `err` bị vứt → `raw` rỗng → parse tài liệu rỗng **thành công** (không lỗi) → check `SkipError` không kích hoạt → **app chạy với toàn bộ config mặc định/zero** thay vì panic.
- Lỗi type-mismatch trong `Unmarshal` (kiểu YAML vs struct field) cũng bị nuốt → sai config im lặng.

Kết hợp với [#08](08-config-nil-pointer-panic.md), hệ quả là hành vi khởi động khó lường.

## Đề xuất sửa

- Propagate lỗi `os.ReadFile` vào cùng nhánh `if err != nil && !SkipError { panic/log }` đang có.
- Trả về/log lỗi `Unmarshal` thay vì `_ =`. `Unmarshall` nên trả `error` cho `FromYaml`/`FromGoStruct` xử lý.
