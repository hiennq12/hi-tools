# Hướng dẫn sử dụng hi-tools

Nơi ghi lại **cách dùng** từng tool trong monorepo `hi-tools`, viết để đọc lại sau vài tháng vẫn dùng được ngay.
(Thiết kế / spec của tool nằm ở thư mục `<tool>/` cạnh đây — `hi-llm/README.md`, `hi-llm/SPEC-phase1.md`...)

## Danh sách tool

| Tool | Trạng thái | Hướng dẫn |
|---|---|---|
| `hi-llm` — quản lý tập trung nhiều LLM provider, key mã hoá | Phase 1 (add/ls/models/env) | [hi-llm/](hi-llm/README.md) |
| `hi-brains` — note kỹ thuật + ôn tập spaced repetition | chưa triển khai | — |

## Build & cài chung

Chạy ở gốc repo `hi-tools/`:

```sh
make build            # build mọi binary vào ./bin
make build hi-llm     # chỉ build một tool
make install          # copy ./bin/* vào $GOBIN (thường ~/go/bin — cần có trong PATH)
make test             # go test ./...
```

## Quy ước khi thêm hướng dẫn cho tool mới

Mỗi tool một thư mục `guides/<tool>/`, gồm 3 file:

| File | Nội dung |
|---|---|
| `README.md` | Bắt đầu từng bước: build → lần dùng đầu tiên → các tình huống hay gặp |
| `commands.md` | Tra cứu từng lệnh: cú pháp, flag, ví dụ, output mẫu, exit code |
| `troubleshooting.md` | Lỗi hay gặp → nguyên nhân → cách xử lý; giới hạn hiện tại |

- Output mẫu lấy từ **binary thật**, không viết tay.
- Thêm/đổi lệnh hay flag → cập nhật `commands.md` trong cùng commit.
- Thêm dòng vào bảng "Danh sách tool" ở trên.
