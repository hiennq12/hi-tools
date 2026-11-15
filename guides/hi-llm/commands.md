# hi-llm — tra cứu lệnh (Phase 2)

```
hi-llm add [--name N] [--base-url URL] [--kind openai]
           [--api-key-stdin | --api-key-env VAR] [--force] [--skip-verify]
hi-llm ls
hi-llm models [provider] [--refresh] [--ttl DURATION] [--json]
hi-llm env <provider> [--style openai|anthropic]
hi-llm ping [provider...] [--chat MODEL]
hi-llm role [role [value] [--force]]
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
| `HI_LLM_ROLES` | đường dẫn `roles.conf` cho `hi-llm role` | cạnh binary `flow` trong `PATH` |

---

## `hi-llm add` — thêm / ghi đè provider

| Flag | Ý nghĩa |
|---|---|
| `--name N` | tên provider: `[a-z0-9._-]`, bắt đầu bằng chữ/số |
| `--base-url URL` | `http`/`https`, có host; path rỗng hoặc `/` → tự nối `/v1`; bỏ `/` cuối |
| `--kind openai` | Phase 1 chỉ có `openai` (mặc định) |
| `--api-key-stdin` | đọc key từ stdin (dòng đầu) → lưu `enc:<name>` trong `keys.enc` |
| `--api-key-env VAR` | chỉ lưu con trỏ `env:VAR`, không tạo `keys.enc` |
| `--force` | ghi đè provider cùng tên (không có terminal thì bắt buộc khi trùng tên); **giữ nguyên `headers` cũ** |
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
| `PING` | kết quả `ping` gần nhất: `✅ 320ms (5m)` / `❌ auth (2h)`; `-` = chưa ping (subscription luôn `-`) |

```
$ HI_LLM_MASTER_PASSWORD='...' hi-llm ls
NAME       KIND    BASE_URL                         KEY                MODELS    PING
claude     sub     claude                           -                  -         -
deeprelay  openai  https://relay.example.com/v1     enc ✅              3 (11s)   ✅ 320ms (5m)
opencode   openai  https://opencode.example.com/v1  env:OC_KEY ❌ thiếu  -        ❌ auth (2h)
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

## Header riêng của provider

Một số relay bắt buộc header phụ (vd opencode-go cần `x-opencode-session`). Khai trong provider:

```yaml
providers:
  opencode-go:
    kind: openai
    base_url: https://opencode.ai/zen/go/v1
    api_key: enc:opencode-go
    headers:
      x-opencode-session: 3f2c...-uuid
```

- Gửi kèm **mọi** request của provider: `GET /models`, `POST /chat/completions`, và bước verify của `add`.
- `add --force` (hoặc xác nhận ghi đè) **giữ nguyên** `headers` cũ và verify bằng chính header đó.
- Không được khai `Authorization` (không phân biệt hoa thường) — key luôn đi qua `api_key`.
  Tên header rỗng / có khoảng trắng / có `:` → lỗi validate.
- `env` không xuất header (chỉ in biến môi trường OpenAI/Anthropic).

## `hi-llm ping` — kiểm provider còn sống

| Cách gọi | Hành vi |
|---|---|
| `ping` | ping **mọi** provider trong registry (bỏ qua subscription) |
| `ping <p> [<p>...]` | chỉ các provider đó; tên lạ → exit 2; subscription → `subscription không ping được` exit 2 |
| `ping <p> --chat MODEL` | gọi `POST {base_url}/chat/completions` với `MODEL` (id đúng như `hi-llm models`) |

- Mặc định ping bằng `GET {base_url}/models`; thành công → ✅ và **ghi cache model** như `models --refresh`.
- `--chat` chỉ dùng với **đúng một** provider; không ghi cache model.
- Các provider ping **song song** nhưng in theo thứ tự tên. Key được resolve tuần tự trước khi ping,
  nên nhiều provider `enc:` chỉ hỏi master password **một lần** mỗi lần chạy.
- Exit `0` khi mọi provider ✅; `1` khi có ít nhất một ❌. Registry rỗng → in hướng dẫn `hi-llm add`, exit 0.

```
$ hi-llm ping
NAME         STATUS  LATENCY  DETAIL
deeprelay    ✅      320ms    30 models
opencode-go  ❌      -        auth: HTTP 401 Unauthorized
```

| `DETAIL` khi ❌ | Khi nào |
|---|---|
| `auth: ...` | HTTP 401/403 |
| `quota: ...` | HTTP 402/429 |
| `http: ...` | HTTP khác 200 còn lại |
| `net: ...` | lỗi mạng / timeout / DNS |
| `response: ...` | HTTP 200 nhưng body không parse được |
| `key: ...` | không lấy được key (env chưa đặt, sai password, chưa có `keys.enc`) — không gửi request, `LATENCY` = `-` |

Kết quả lưu ở `$HI_LLM_CACHE_DIR/_ping.json` (gộp, ping một provider không xoá kết quả provider khác)
để `ls` hiện cột `PING`. Lỗi không bao giờ chứa api key.

## `hi-llm role` — chọn model cho my-flow

my-flow đọc `roles.conf` dạng `role=harness:model` (dòng cuối của role thắng):

```
implement=opencode:opencode-go/deepseek-v4.1-flash
review=agy:gemini-3.8-flash-high
```

Vị trí file: `HI_LLM_ROLES` nếu đặt → không thì cạnh binary `flow` trong `PATH`
(`<thư mục>/roles.conf`). Không tìm được → lỗi nêu cách đặt `HI_LLM_ROLES`.

| Cách gọi | Hành vi |
|---|---|
| `role` | in bảng `ROLE | VALUE` theo thứ tự trong file (role trùng lấy dòng cuối) |
| `role <role> <value>` | đặt trực tiếp, không cần TTY; `<value>` phải có trong candidates |
| `role <role> <value> --force` | cho model chưa có trong cache, nhưng vẫn phải dạng `harness:model` |
| `role <role>` | TTY: mở `fzf` chọn trong candidates |

Candidates = mỗi provider có cache model × mỗi harness trong `harnesses` (mặc định `[opencode]`)
→ `<harness>:<provider>/<model>`, cộng `<harness>:<model>` từ `models` khai cho subscription.
Sắp xếp, bỏ trùng. Rỗng → nhắc chạy `hi-llm models <provider>` hoặc khai `models` cho subscription.

- `<role>` phải **đã có** trong `roles.conf` (exit 2 nêu đường dẫn file).
- Ghi: thay **mọi** dòng `^<role>=`, giữ nguyên comment/dòng trống/thứ tự/dấu xuống dòng cuối và quyền file.
- Giá trị không đổi → in `<role>: không đổi`, không ghi. Huỷ fzf (exit 1/130) → `đã huỷ`, exit 1.

```
$ hi-llm role
ROLE       VALUE
implement  opencode:deeprelay/deepseek-v4.1-flash
review     agy:gemini-3.8-flash-high

$ hi-llm role implement opencode:deeprelay/glm-5.3
implement: opencode:deeprelay/deepseek-v4.1-flash → opencode:deeprelay/glm-5.3
```
