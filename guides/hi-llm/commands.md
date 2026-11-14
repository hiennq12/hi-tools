# hi-llm — tra cứu lệnh (Phase 1)

```
hi-llm add [--name N] [--base-url URL] [--kind openai]
           [--api-key-stdin | --api-key-env VAR] [--force] [--skip-verify]
hi-llm ls
hi-llm models [provider] [--refresh] [--ttl DURATION] [--json]
hi-llm env <provider> [--style openai|anthropic]
```

- `hi-llm` không tham số → in usage. `hi-llm <lệnh> -h` → liệt kê flag.
- Flag đặt trước hay sau tên provider đều được: `models deeprelay --refresh` ≡ `models --refresh deeprelay`.
- Exit code: `0` thành công · `2` sai cú pháp / dữ liệu không hợp lệ · `1` lỗi lúc chạy (mạng, password, file).

## Biến môi trường

| Biến | Tác dụng | Mặc định |
|---|---|---|
| `HI_LLM_HOME` | thư mục `providers.yaml` + `keys.enc` | `~/.config/hi-llm` |
| `HI_LLM_CACHE_DIR` | thư mục cache model | `~/.cache/hi-llm` |
| `HI_LLM_MASTER_PASSWORD` | master password, khỏi gõ (script, `ls` hiện ✅/❌) | — (hỏi qua terminal) |

---

## `hi-llm add` — thêm / ghi đè provider

| Flag | Ý nghĩa |
|---|---|
| `--name N` | tên provider: `[a-z0-9._-]`, bắt đầu bằng chữ/số |
| `--base-url URL` | `http`/`https`, có host; path rỗng hoặc `/` → tự nối `/v1`; bỏ `/` cuối |
| `--kind openai` | Phase 1 chỉ có `openai` (mặc định) |
| `--api-key-stdin` | đọc key từ stdin (dòng đầu) → lưu `enc:<name>` trong `keys.enc` |
| `--api-key-env VAR` | chỉ lưu con trỏ `env:VAR`, không tạo `keys.enc` |
| `--force` | ghi đè provider cùng tên (không có terminal thì bắt buộc khi trùng tên) |
| `--skip-verify` | không gọi thử `GET {base_url}/models` |

`--api-key-stdin` và `--api-key-env` không dùng cùng lúc. Thiếu flag mà có terminal → wizard hỏi phần còn thiếu
(xem [README bước 2](README.md#2-thêm-provider-đầu-tiên)).

Thứ tự: validate → gọi thử `/models` → ghi key vào `keys.enc` → ghi `providers.yaml`. Bước nào lỗi thì **không ghi gì**.

```
$ printf '%s' "$KEY" | hi-llm add --name deeprelay --base-url https://relay.example.com --api-key-stdin
đã thêm provider deeprelay (https://relay.example.com/v1)

$ OC_KEY=sk-... hi-llm add --name opencode --base-url https://opencode.example.com --api-key-env OC_KEY
đã thêm provider opencode (https://opencode.example.com/v1)

$ printf x | hi-llm add --name deeprelay --base-url https://relay.example.com --api-key-stdin
hi-llm add: provider "deeprelay" đã tồn tại, dùng --force để ghi đè          [exit 2]

$ printf x | hi-llm add --name bad --base-url http://127.0.0.1:1 --api-key-stdin
cảnh báo: xác minh http://127.0.0.1:1/v1/models thất bại: ... connection refused   [exit 1]
```

## `hi-llm ls` — bảng tổng quan

Không flag. **Không bao giờ hỏi password, không in secret.**

| Cột | Giá trị |
|---|---|
| `KIND` | `openai` (provider) / `sub` (subscription) |
| `BASE_URL` | URL provider; với subscription là tên harness |
| `KEY` | `enc ✅` mở được · `enc ❌` sai password / chưa có key · `enc 🔒` chưa đặt `HI_LLM_MASTER_PASSWORD` · `env:VAR ✅` / `env:VAR ❌ thiếu` |
| `MODELS` | `<số model> (<tuổi cache>)`, `-` = chưa có cache |

```
$ HI_LLM_MASTER_PASSWORD='...' hi-llm ls
NAME       KIND    BASE_URL                         KEY                MODELS
claude     sub     claude                           -                  -
deeprelay  openai  https://relay.example.com/v1     enc ✅              3 (11s)
opencode   openai  https://opencode.example.com/v1  env:OC_KEY ❌ thiếu  -
```

Registry rỗng → in hướng dẫn chạy `hi-llm add`.

## `hi-llm models` — danh sách model

| Cách gọi | Hành vi |
|---|---|
| `models <provider>` | cache còn hạn (mặc định 1h) → in từ cache; hết hạn → gọi `GET {base_url}/models` rồi ghi cache |
| `models <provider> --refresh` | bỏ qua cache, luôn gọi mạng |
| `models <provider> --ttl 10m` | đổi hạn cache (`30s`, `10m`, `2h`...) |
| `models ... --json` | in JSON `[{"provider":..,"id":..}]` |
| `models` (không provider) | gộp mọi provider **chỉ từ cache**, không gọi mạng; provider chưa có cache → cảnh báo, vẫn exit 0 |

`--refresh`/`--ttl` mà không có provider → lỗi. Subscription không có `models`.

```
$ hi-llm models deeprelay
deeprelay/deepseek-v4.1-flash
deeprelay/glm-5.3
deeprelay/qwen3.8-max

$ hi-llm models deeprelay --json
[{"provider":"deeprelay","id":"deepseek-v4.1-flash"},{"provider":"deeprelay","id":"glm-5.3"},...]

$ hi-llm models
cảnh báo: provider opencode chưa có cache models
deeprelay/deepseek-v4.1-flash
...
```

Cache: `~/.cache/hi-llm/<provider>.json` = `{"fetched_at": "...", "models": [...]}`. Xoá file = xoá cache.

## `hi-llm env` — in biến môi trường

| Style | In ra |
|---|---|
| `openai` (mặc định) | `OPENAI_API_KEY`, `OPENAI_BASE_URL` (= base_url) |
| `anthropic` | `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_BASE_URL` (= base_url bỏ `/v1` cuối, vì SDK Anthropic tự nối `/v1/messages`) |

Lệnh **duy nhất** in secret (ra stdout). Giá trị trong nháy đơn, an toàn với `eval`.

```
$ hi-llm env deeprelay
export OPENAI_API_KEY='sk-...'
export OPENAI_BASE_URL='https://relay.example.com/v1'

$ hi-llm env deeprelay --style anthropic
export ANTHROPIC_AUTH_TOKEN='sk-...'
export ANTHROPIC_BASE_URL='https://relay.example.com'

$ eval "$(hi-llm env deeprelay)"      # dùng thật: luôn có ngoặc kép
```

Subscription → `provider này là subscription CLI, không có env` (exit 2).
