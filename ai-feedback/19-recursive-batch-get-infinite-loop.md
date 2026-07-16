# 19 — `RecursiveBatchGetItem` có thể loop vô hạn (latent)

- **Mức độ:** 🟡 Medium (hiện chưa được gọi — latent)
- **Vùng:** Repository / pkg dynamodb
- **Trạng thái:** ⬜ Chưa xử lý
- **File liên quan:**
  - [pkg/dynamodb/recursive_batch_get_item.go](../pkg/dynamodb/recursive_batch_get_item.go) (dòng 28-47)
  - So sánh bản đúng: [pkg/dynamodb/recursive_batch_write_item.go](../pkg/dynamodb/recursive_batch_write_item.go) (dòng 25-32)

## Mô tả

Khác với `RecursiveBatchWriteItem` (có `counter++` ngay sau check depth), `RecursiveBatchGetItem`:
1. **Không** tăng `counter` → `if counter > depth { break }` không bao giờ đúng → tham số `depth` là dead code.
2. Khi `output.Responses[tableName]` vắng (cả chunk bị throttle, không có gì trả về), code `continue` **trước** dòng `unprocessed = output.UnprocessedKeys` → `unprocessed` không được cập nhật → **retry y hệt request mãi mãi**, không backoff, không kiểm `ctx.Done()`.

→ Hiện chưa method repo nào gọi `RecursiveBatchGetItem`, nhưng nó là public API ([0_interactor.go](../pkg/dynamodb/0_interactor.go)) và comment bảng (vd `noti_content`: "BatchGetItem by IDs") cho thấy sắp dùng. Ai wire method BatchGet đầu tiên dưới throttling kéo dài sẽ treo goroutine.

## Đề xuất sửa

- Thêm `counter++` đúng vị trí như bản write.
- Đảm bảo `unprocessed = output.UnprocessedKeys` luôn chạy trước mọi `continue`.
- Thêm kiểm `ctx.Done()` mỗi vòng + backoff, cho đồng bộ với bản write.

## Ghi chú review
> _(chỗ trống để bạn ghi quyết định)_
