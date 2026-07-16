# 21 — Độ phủ test thấp

- **Mức độ:** 🟢 Low
- **Trạng thái tổng:** ⬜ Chưa xử lý

12 file `_test.go` / 303 file Go. Không có integration test chạy Postgres/DynamoDB thật (test migration chỉ kiểm logic Go thuần).
→ Bổ sung integration test (repo có sẵn skill `golang-unit-test-generator` + testcontainers) cho: rate limiter (bắt deadlock #01), repository parity (#04, #18), migration chạy DB thật, pubsub reconnect (#10).
