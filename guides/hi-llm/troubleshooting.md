# hi-llm — xử lý lỗi & giới hạn

## Lỗi hay gặp

| Thông báo | Nguyên nhân | Cách xử lý |
|---|---|---|
| `thiếu api key, dùng --api-key-stdin hoặc --api-key-env VAR` | chạy `add` không có terminal (script, pipe) và chưa chỉ cách lấy key | thêm `--api-key-stdin` (key qua stdin) hoặc `--api-key-env VAR` |
| `cần HI_LLM_MASTER_PASSWORD hoặc terminal để nhập master password` | key dạng `enc:` mà không có terminal để hỏi password | `export HI_LLM_MASTER_PASSWORD=...` trong script đó |
| `master password sai hoặc keys.enc hỏng` | gõ sai password, hoặc file `keys.enc` bị sửa/hỏng | gõ lại đúng password. File **không** bị ghi đè khi lỗi này xảy ra |
| `master password không được rỗng` | tạo `keys.enc` mới với password rỗng | đặt password khác rỗng |
| `master password nhập lại không khớp` | 2 lần gõ khác nhau khi tạo `keys.enc` | chạy lại `add` |
| `chưa có keys.enc, chạy hi-llm add` | registry có `enc:<name>` nhưng `keys.enc` đã bị xoá / chưa tạo | `hi-llm add --name <name> ... --force` để lưu lại key |
| `biến môi trường X chưa đặt` | provider dạng `env:X` mà biến `X` trống trong shell hiện tại | `export X=...` rồi chạy lại |
| `provider "x" đã tồn tại, dùng --force để ghi đè` | trùng tên khi không có terminal | thêm `--force` |
| `tên "X" không hợp lệ (chỉ [a-z0-9._-], ...)` | tên có chữ hoa, `/`, dấu cách... | dùng chữ thường, số, `.` `_` `-` |
| `base_url "..." phải dùng http hoặc https` | URL sai scheme / thiếu host | sửa URL |
| `cảnh báo: xác minh .../models thất bại: ...` | sai key, sai URL, relay đang lỗi (401, timeout...) | kiểm tra lại; chắc chắn đúng thì `--skip-verify` |
| `không có provider "x"` | gõ sai tên | `hi-llm ls` để xem tên đúng |
| `provider này là subscription CLI, không có env` | gọi `env`/`models` với subscription | subscription dùng qua CLI của nó, không có key |
| `--refresh/--ttl chỉ dùng kèm provider` | `hi-llm models --refresh` không có tên | `hi-llm models <provider> --refresh` |
| `.../providers.yaml: yaml không hợp lệ: yaml: line N: ...` (hoặc lỗi validate kèm đường dẫn) | sửa tay yaml sai cú pháp / sai quy tắc | mở file theo đường dẫn trong lỗi, sửa dòng N |
| `ls` hiện `enc 🔒` | bình thường — `ls` không hỏi password | `HI_LLM_MASTER_PASSWORD=... hi-llm ls` để kiểm key |

## Các việc chưa có lệnh (làm tay)

| Việc | Cách làm tạm |
|---|---|
| **Quên master password** | không khôi phục được: `rm ~/.config/hi-llm/keys.enc` rồi `hi-llm add --force` lại từng provider `enc:` |
| Đổi master password | như trên (xoá `keys.enc`, add lại với password mới) |
| Xoá provider | xoá mục đó trong `providers.yaml`. Key cũ vẫn nằm trong `keys.enc` (vô hại); muốn sạch hẳn thì làm như "quên password" |
| Đổi base_url | `hi-llm add --name <tên> --base-url <mới> ... --force` |
| Thêm subscription | sửa tay `providers.yaml` (xem [README bước 6](README.md#6-khai-báo-subscription-sửa-tay)) |
| Xoá cache model | `rm ~/.cache/hi-llm/<provider>.json` hoặc `models <provider> --refresh` |

## Giới hạn Phase 1 / việc còn tồn

- Chỉ provider OpenAI-compatible (`GET /models` trả `{"data":[{"id":...}]}`).
- Mỗi lệnh cần key `enc:` đều hỏi master password (chưa cache trong phiên) — dùng `HI_LLM_MASTER_PASSWORD` nếu gọi nhiều lần.
- Chưa có: `ping`, `use` (fzf), `defaults`, `sync` sang opencode/commandcode, `role` cho my-flow, backend `plain:`/`keychain:`.
- Đã biết, chưa sửa (ghi trong todos của repo):
  - sửa tay `base_url` có `/` cuối (vd `.../v1/`) → `models` gọi sai đường dẫn; dùng `add --force` thay vì sửa tay.
  - lỗi HTTP in lặp mã (`HTTP 401 401 Unauthorized`).
  - `add`/`ls` lờ đi tham số thừa (vd `hi-llm add deeprelay` quên `--name` → wizard hỏi tên như bình thường).
  - `env <p> --style sai` hỏi password trước rồi mới báo style sai.
