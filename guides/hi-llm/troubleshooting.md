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
| `biến môi trường X chưa đặt` | provider dạng `env:X` mà biến `X` trống trong shell hiện tại (`add` có terminal thì hỏi `Vẫn lưu provider?`) | `export X=...` rồi chạy lại |
| `tham số thừa [...]` | gõ tên provider không kèm `--name` (`hi-llm add deeprelay`) hoặc `ls` có tham số | `hi-llm add --name deeprelay ...` |
| `provider "x" đã tồn tại, dùng --force để ghi đè` | trùng tên khi không có terminal | thêm `--force` |
| `tên "X" không hợp lệ (chỉ [a-z0-9._-], ...)` | tên có chữ hoa, `/`, dấu cách... | dùng chữ thường, số, `.` `_` `-` |
| `base_url "..." phải dùng http hoặc https` | URL sai scheme / thiếu host | sửa URL |
| `cảnh báo: xác minh .../models thất bại: ...` | sai key, sai URL, relay đang lỗi (401, timeout...) | kiểm tra lại; chắc chắn đúng thì `--skip-verify` |
| `không có provider "x"` | gõ sai tên | `hi-llm ls` để xem tên đúng |
| `provider này là subscription CLI, không có env` | gọi `env`/`models` với subscription | subscription dùng qua CLI của nó, không có key |
| `--refresh/--ttl chỉ dùng kèm provider` | `hi-llm models --refresh` không có tên | `hi-llm models <provider> --refresh` |
| `.../providers.yaml: yaml không hợp lệ: yaml: line N: ...` (hoặc lỗi validate kèm đường dẫn) | sửa tay yaml sai cú pháp / sai quy tắc | mở file theo đường dẫn trong lỗi, sửa dòng N |
| `provider "x": header "..." không hợp lệ` | khai `headers` là `Authorization`, tên rỗng, có khoảng trắng hoặc `:` | bỏ `Authorization` (key đi qua `api_key`), sửa tên header |
| `ls` hiện `enc 🔒` | bình thường — `ls` không hỏi password | `HI_LLM_MASTER_PASSWORD=... hi-llm ls` để kiểm key |
| `❌ key: ...` trong `ping` | không lấy được key: env chưa đặt, sai master password, chưa có `keys.enc` | `export` biến env hoặc `HI_LLM_MASTER_PASSWORD`; kiểm lại `add` |
| `❌ auth: HTTP 401/403` trong `ping` | sai key hoặc thiếu header riêng của provider | sửa key; khai `headers:` (vd `x-opencode-session`) rồi ping lại |
| `❌ quota: HTTP 402/429` trong `ping` | hết credit / bị rate limit | nạp quota hoặc chờ; đổi provider cho role |
| `❌ net: ...` trong `ping` | lỗi mạng / timeout / DNS | kiểm mạng, base_url; thử `curl` cùng URL |
| `❌ response: ...` trong `ping` | HTTP 200 nhưng body `/models` không đúng chuẩn OpenAI | kiểm base_url/relay |
| `subscription không ping được` | `hi-llm ping <subscription>` | ping chỉ áp dụng provider API; subscription kiểm bằng CLI của nó |
| `--chat chỉ dùng khi chỉ định đúng một provider` | `ping --chat` không kèm hoặc kèm nhiều provider | `hi-llm ping <provider> --chat <model>` |
| `không tìm thấy roles.conf, đặt HI_LLM_ROLES` | không có env và không có `flow` trong `PATH` | `export HI_LLM_ROLES=/path/roles.conf` |
| `role "x" không có trong <file>` | gõ role chưa khai trong `roles.conf` | thêm dòng `x=...` vào file rồi chạy lại |
| `"..." không có trong danh sách model, dùng --force` | model chưa có trong cache / subscription chưa khai | `hi-llm models <provider> --refresh` rồi chọn lại, hoặc `--force` |
| `chưa có model nào: ...` | chưa có cache model và subscription không khai `models` | chạy `hi-llm models <provider>` hoặc thêm `models:` cho subscription |
| `cần TTY để chọn, hoặc truyền <value>` | không có terminal hoặc thiếu `fzf` | truyền thẳng `hi-llm role <role> <harness:model>` |
| `đã huỷ` | thoát fzf mà không chọn (exit 1/130) | không sao — `roles.conf` không bị ghi |
| `--force cần value dạng harness:model` | value thiếu `:` hoặc một phần rỗng | dùng `harness:model`, vd `commandcode:zai-org/glm-5.3` |
| (curl sau `env`) `MissingSessionID ... x-opencode-session` | provider **opencode-go** bắt buộc header session | khai `headers: {x-opencode-session: ...}` trong `providers.yaml` (dùng cho `ping`/`models`), hoặc thêm `-H` nếu curl tay |
| (curl sau `env`) `content: null`, `finish_reason: "length"` | model có reasoning tiêu hết `max_tokens` để nghĩ | tăng `max_tokens` (vd 200+) |

## Các việc chưa có lệnh (làm tay)

| Việc | Cách làm tạm |
|---|---|
| **Quên master password** | không khôi phục được: `rm ~/.config/hi-llm/keys.enc` rồi `hi-llm add --force` lại từng provider `enc:` |
| Đổi master password | như trên (xoá `keys.enc`, add lại với password mới) |
| Xoá provider | xoá mục đó trong `providers.yaml`. Key cũ vẫn nằm trong `keys.enc` (vô hại); muốn sạch hẳn thì làm như "quên password" |
| Đổi base_url | `hi-llm add --name <tên> --base-url <mới> ... --force` |
| Thêm subscription | sửa tay `providers.yaml` (xem [README bước 6](README.md#6-khai-báo-subscription-sửa-tay)) |
| Xoá cache model | `rm ~/.cache/hi-llm/<provider>.json` hoặc `models <provider> --refresh` |

## Giới hạn hiện tại / việc còn tồn

- Chỉ provider OpenAI-compatible (`GET /models` trả `{"data":[{"id":...}]}`).
- Mỗi lần chạy chỉ hỏi master password một lần (kể cả `ping` nhiều provider `enc:`), nhưng **chưa cache giữa các lệnh** — dùng `HI_LLM_MASTER_PASSWORD` nếu gọi nhiều lần.
- Header `{uuid}` chưa tự sinh — `headers` là giá trị cố định, sửa tay trong `providers.yaml`.
- Chưa có: `use` (fzf), `defaults`, `sync` sang opencode/commandcode, backend `plain:`/`keychain:`, cảnh báo quota định kỳ.
