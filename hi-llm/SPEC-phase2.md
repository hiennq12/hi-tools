# SPEC: hi-llm Phase 2 — `ping` + header riêng của provider + `role` cho my-flow

## Bối cảnh

Phase 1 đã có: registry `providers.yaml`, keystore `enc`/`env`, các lệnh `add`, `ls`, `models`, `env`
(xem `SPEC-phase1.md`, code ở `cmd/hi-llm/` + `internal/llm/`). Phase 2 thêm:

1. **`hi-llm ping`** — kiểm provider còn sống / sai key / hết quota, kèm độ trễ; kết quả lưu lại để `ls` hiện.
2. **`headers:` trong provider** — header HTTP riêng (vd opencode-go bắt buộc `x-opencode-session`), gửi kèm mọi request.
3. **`hi-llm role`** — chọn model cho từng role của my-flow (fzf) và sửa dòng tương ứng trong `roles.conf`.

Quyết định đã chốt (không làm trong phase này): **không** cache master password trong phiên
(`HI_LLM_MASTER_PASSWORD` đã thay được); **bỏ** lệnh `use` và mục `defaults` (`role` là nơi tiêu thụ thật duy nhất;
làm khi có nơi đọc `defaults`); `sync` sang opencode/commandcode vẫn ở Phase 3.

Mọi **Quy tắc chung** của `SPEC-phase1.md` vẫn áp dụng: `run(args, stdin, stdout, stderr) int`, không hỏi khi
không có TTY, override đường dẫn bằng `HI_LLM_HOME`/`HI_LLM_CACHE_DIR`/`HI_LLM_MASTER_PASSWORD`, ghi file atomic,
flag đặt trước/sau tham số vị trí đều được, không in secret, test không dùng network thật (`httptest`) và không đụng `~`.
**Không thêm dependency** (go.mod giữ nguyên). Tham số vị trí thừa → lỗi exit 2 (như `add`/`ls` hiện tại).

## 1. Header riêng của provider

```yaml
providers:
  opencode-go:
    kind: openai
    base_url: https://opencode.ai/zen/go/v1
    api_key: enc:opencode-go
    headers:                      # tuỳ chọn, sửa tay
      x-opencode-session: 3f2c...-uuid
```

- `registry.Provider` thêm `Headers map[string]string` (`yaml:"headers,omitempty"`).
- Validate: tên header không rỗng, không chứa khoảng trắng hay `:`; **không được là `Authorization`**
  (so sánh không phân biệt hoa thường) — key luôn đi qua `api_key`.
- `provider.Client` thêm field `Headers map[string]string`; mọi request (`/models`, `/chat/completions`) gửi kèm.
  Nơi gọi tạo client theo provider: `c := provider.NewClient(); c.Headers = p.Headers`.
- `models <p>` và bước verify của `add` dùng header của provider.
- `add --force` (hoặc xác nhận ghi đè) trên provider đã có: **giữ nguyên `headers` cũ**, và verify dùng các header đó.
- `env` không đổi (không xuất header).

## 2. `hi-llm ping [provider...] [--chat MODEL]`

- Không tham số → ping **mọi provider** trong registry (bỏ qua subscription). Có tham số → chỉ các provider đó;
  tên không tồn tại → lỗi exit 2; tên là subscription → lỗi `subscription không ping được` exit 2.
- Mặc định: `GET {base_url}/models` (Bearer key + headers). Thành công (HTTP 200, parse được) → ✅,
  đồng thời **ghi luôn cache model** của provider đó (như `models --refresh`).
- `--chat MODEL`: chỉ dùng khi chỉ định **đúng một** provider (khác → lỗi exit 2). Gửi
  `POST {base_url}/chat/completions` body `{"model":MODEL,"messages":[{"role":"user","content":"ping"}],"max_tokens":16}`.
  HTTP 200 → ✅ (không xét nội dung: model reasoning có thể trả `content: null`). `MODEL` là id đúng như API
  (phần sau `<provider>/` trong output `hi-llm models`). Không ghi cache model ở chế độ này.
- Ping các provider **song song**, in kết quả theo thứ tự tên:

```
NAME         STATUS  LATENCY  DETAIL
deeprelay    ✅      320ms    30 models
opencode-go  ❌      -        auth: HTTP 401 Unauthorized
```

  - `LATENCY`: thời gian request tính bằng ms (`-` khi lỗi trước khi gửi, vd không lấy được key).
  - `DETAIL` khi ✅: `<n> models` (mặc định) hoặc `chat ok` (`--chat`). Khi ❌: `<loại>: <lỗi>`.
- Loại lỗi (`provider.Classify`):

| Loại | Khi nào |
|---|---|
| `auth` | HTTP 401, 403 |
| `quota` | HTTP 402, 429 |
| `http` | HTTP khác 200 còn lại |
| `net` | lỗi mạng / timeout / DNS (không có response) |
| `response` | HTTP 200 nhưng body không parse được |
| `key` | không lấy được key (biến env chưa đặt, sai master password, chưa có `keys.enc`...) — không gửi request |

- Key: resolve **tuần tự trước khi ping** (không hỏi password song song). Nhiều provider `enc:` → chỉ hỏi
  master password **một lần** trong lần chạy (`keystore.OncePassword`). Không TTY, không env password →
  provider `enc:` báo ❌ `key`, các provider khác vẫn ping.
- Exit code: 0 khi mọi provider ✅; 1 khi có ít nhất một ❌. Registry không có provider → in hướng dẫn
  `hi-llm add`, exit 0.
- Lưu kết quả vào `$HI_LLM_CACHE_DIR/_ping.json` (tên bắt đầu `_` nên không trùng cache của provider nào):
  map `tên → {at, ok, latency_ms, kind, error}`; **gộp** với nội dung cũ (ping một provider không xoá kết quả
  của provider khác). File hỏng → coi như rỗng. Chuỗi lỗi không bao giờ chứa key.

### `ls` thêm cột `PING`

`NAME | KIND | BASE_URL | KEY | MODELS | PING`. Đọc `_ping.json`, không gọi mạng:
`✅ 320ms (5m)` / `❌ auth (2h)` (tuổi theo `formatAge`) / `-` khi chưa ping. Subscription: `-`.

## 3. `hi-llm role`

my-flow đọc `roles.conf` dạng `role=harness:model` (dòng cuối cùng của role thắng), ví dụ:

```
implement=opencode:opencode-go/deepseek-v4.1-flash
review=agy:gemini-3.8-flash-high
```

### Vị trí `roles.conf`

1. env `HI_LLM_ROLES` (đường dẫn file) nếu đặt — test bắt buộc dùng cách này;
2. không có → tìm `flow` trong `PATH` (`exec.LookPath`), resolve symlink (`filepath.EvalSymlinks`), lấy
   `<thư mục>/roles.conf` (đúng chỗ my-flow đọc: `$FLOW_HOME/roles.conf`);
3. không tìm được → lỗi `không tìm thấy roles.conf, đặt HI_LLM_ROLES`.

### Danh sách lựa chọn (candidates)

- Mỗi provider có cache model (bỏ qua TTL), với **mỗi harness** trong `harnesses` của provider:
  `<harness>:<provider>/<model>` — chuỗi sau `<harness>:` trùng đúng dòng của `hi-llm models`.
  `harnesses` là danh sách tuỳ chọn trong yaml, không khai → `[opencode]`:

```yaml
providers:
  deeprelay:
    kind: openai
    base_url: https://.../v1
    api_key: enc:deeprelay
    harnesses: [opencode, commandcode]   # harness nào đã khai provider này
```

  `registry.Provider` thêm `Harnesses []string` (`yaml:"harnesses,omitempty"`); validate: mỗi phần tử khớp regex
  tên (`ValidName`). Cách đặt tên model của commandcode cho provider tự thêm do Phase 3 (`sync`) chốt; khác
  `<provider>/<model>` thì dùng `--force`.
- Mỗi subscription khai `models:` trong yaml: `<harness>:<model>`.

```yaml
subscriptions:
  claude:      { harness: claude, models: [claude-opus-5-5, claude-sonnet-5-5] }
  agy:         { harness: agy, models: [gemini-3.8-flash-high, gemini-3.8-flash-medium] }
  commandcode: { harness: commandcode, models: [zai-org/glm-5.3] }
```

  `registry.Subscription` thêm `Models []string` (`yaml:"models,omitempty"`).
- Sắp xếp, bỏ trùng. Rỗng → lỗi `chưa có model nào: chạy hi-llm models <provider> hoặc khai models cho subscription`.

### Lệnh

```
hi-llm role                         # bảng ROLE | VALUE theo roles.conf hiện tại
hi-llm role <role> <value> [--force] # đặt trực tiếp, không cần TTY
hi-llm role <role>                   # TTY: mở fzf chọn trong candidates
```

- `<role>` phải **đã có** trong `roles.conf` (tránh gõ nhầm tạo role lạ) → không có: lỗi exit 2 nêu file.
- `<value>` phải nằm trong candidates; không có → lỗi exit 2 gợi ý `--force` (cho model chưa có trong cache).
  `--force` vẫn bắt buộc dạng `harness:model` (có `:`, hai phần không rỗng).
- Không `<value>`: cần TTY và `fzf` trong `PATH`; thiếu → lỗi nêu cách truyền `<value>`. Gọi
  `fzf --prompt "<role>> "` với candidates qua stdin, đọc lựa chọn từ stdout. Thoát fzf không chọn
  (exit 1/130) → `đã huỷ`, exit 1, không ghi gì.
- Ghi: thay **mọi** dòng `^<role>=` bằng `<role>=<value>`, giữ nguyên các dòng khác (kể cả comment, dòng trống,
  thứ tự) và dấu xuống dòng cuối file; ghi atomic, **giữ quyền file cũ**. In `<role>: <cũ> → <mới>`.
  Giá trị không đổi → in `<role>: không đổi`, không ghi file.
- Đọc bảng (`hi-llm role` không tham số): mỗi role một dòng theo thứ tự xuất hiện, role trùng lấy dòng cuối
  (giống `tail -1` của my-flow); bỏ qua dòng trống và dòng bắt đầu bằng `#`.

## Cấu trúc code

```
cmd/hi-llm/
  main.go       # dispatch thêm ping, role; usage
  ping.go       # lệnh ping
  role.go       # lệnh role: tìm roles.conf, candidates, fzf, ghi file
  ls.go         # thêm cột PING
  add.go        # giữ headers khi ghi đè, verify dùng headers
  models.go     # client dùng headers
internal/llm/
  registry/registry.go   # Provider.Headers, Subscription.Models, validate header
  provider/provider.go   # Client.Headers, Chat, Classify
  provider/ping.go       # PingResult, LoadPings, SavePings (merge)
  keystore/enc.go        # OncePassword
```

## Tiêu chí hoàn thành

- Máy thật (người kiểm ở gate 2):
  - thêm `headers: {x-opencode-session: <uuid>}` cho opencode-go → `hi-llm ping` hiện ✅ cho cả hai provider;
    `hi-llm ping opencode-go --chat <model>` ✅; `hi-llm ls` có cột PING.
  - `hi-llm role implement` mở fzf, chọn → `roles.conf` đổi đúng một dòng; `flow` vẫn đọc được.
- Unit test (`httptest`, `t.TempDir()`, env `HI_LLM_*`, không fzf thật):
  header gửi kèm cả `/models` lẫn chat + validate cấm `Authorization`; `add --force` giữ headers;
  `Classify` đủ 6 loại; ping nhiều provider song song đúng thứ tự + exit code + merge `_ping.json` + ghi cache model;
  `--chat` gửi đúng body và từ chối khi ≠ 1 provider; provider `env:` chưa đặt → ❌ `key` không gửi request;
  `OncePassword` chỉ gọi hàm gốc một lần; `ls` cột PING (có/không có `_ping.json`);
  `role`: candidates theo `harnesses` (mặc định opencode), bảng hiện tại, đặt giá trị hợp lệ, giữ comment/thứ tự/quyền file, role không có, value ngoài candidates,
  `--force`, không TTY mà thiếu value, candidates từ cache + subscription models.
- `go vet` sạch, `go test ./...` pass.

verify: go build ./... && go vet ./... && go test ./...

## Ngoài phạm vi

Cache master password trong phiên, `use`, `defaults`, `sync opencode`/`sync commandcode` (Phase 3),
backend `plain`/`keychain`, cảnh báo quota định kỳ, lệnh xoá/sửa provider, `{uuid}` tự sinh trong header.
