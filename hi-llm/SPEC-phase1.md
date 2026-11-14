# SPEC: hi-llm Phase 1 — registry provider + keystore mã hoá + add/ls/models/env

## Bối cảnh

`hi-llm` là CLI quản lý tập trung nhiều LLM provider và sinh config cho nhiều harness
(xem README.md). Phase 1 xây phần lõi: registry `providers.yaml`, keystore mã hoá
(`keys.enc` + master password), và 4 lệnh `add`, `ls`, `models`, `env`.

Chưa làm trong Phase 1: `ping`, `use` (fzf), harness adapter (`sync`, `role`), health check định kỳ,
cache master password trong session.

Ngôn ngữ: **Go**. Repo: `hi-tools` — monorepo, **module Go chung ở gốc** (`go.mod` đã có sẵn, module `hi-tools`).
Dependency đã được thêm sẵn vào `go.mod`/`go.sum`: `gopkg.in/yaml.v3`, `golang.org/x/crypto`, `golang.org/x/term`.
**Không thêm dependency nào khác** (agent không chạy được `go get`/`go mod tidy`).
Code của tool này nằm ở `cmd/hi-llm/` + `internal/llm/`; hạ tầng CLI dùng chung đặt ở `internal/hix/` (xem mục Cấu trúc code).
Chạy được trên macOS và Linux (không cgo, không phụ thuộc OS keystore ở đường mặc định).

## Quy tắc chung (áp dụng mọi lệnh)

- Mỗi lệnh cài dưới dạng hàm testable: `func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int`
  (exit code). `main` chỉ gọi `os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))`.
- **Không bao giờ hỏi khi không có terminal**: nếu stdin không phải TTY mà còn thiếu giá trị bắt buộc
  (name, base_url, key, master password, xác nhận ghi đè...) → in lỗi nêu flag/env cần dùng, exit code ≠ 0.
  Test luôn chạy ở chế độ này nên không được treo.
- Đường dẫn override bằng env (test bắt buộc dùng, không được đụng `~` thật):
  - `HI_LLM_HOME`: thư mục config (mặc định `~/.config/hi-llm/`).
  - `HI_LLM_CACHE_DIR`: thư mục cache (mặc định `~/.cache/hi-llm/`).
  - `HI_LLM_MASTER_PASSWORD`: master password không interactive.
- Thư mục config tạo với quyền `0700`. Mọi file ghi ra (`providers.yaml`, `keys.enc`, cache) đều ghi
  atomic: file tạm cùng thư mục + `rename`. `keys.enc` quyền `0600`.
- HTTP: stdlib `net/http`, timeout 15s; không network ngoài `GET {base_url}/models`.
- Lỗi in ra stderr, không panic. **Không in secret** ra stdout/stderr, trừ lệnh `env`.

## Phạm vi Phase 1

### Registry — `$HI_LLM_HOME/providers.yaml`

```yaml
providers:
  deeprelay:
    kind: openai
    base_url: https://.../v1
    api_key: enc:deeprelay      # con trỏ keystore: enc: | env: | plain: | keychain:
subscriptions:
  claude: { harness: claude }
  agy:    { harness: agy }
defaults:
  chat: opencode-go/qwen3.8-max
```

- Tên provider/subscription khớp regex `^[a-z0-9][a-z0-9._-]*$` (không `/`, không `..` — tên còn dùng
  làm tên file cache). Tên không được trùng giữa `providers` và `subscriptions`.
- `kind`: Phase 1 chỉ chấp nhận `openai`. Kind khác → lỗi validate.
- `api_key` phải có tiền tố hợp lệ `enc:` | `env:` | `plain:` | `keychain:` và phần sau không rỗng.
- `subscriptions` chỉ được sửa tay trong yaml (lệnh `add` không tạo subscription).
- File chưa tồn tại → coi như registry rỗng. File yaml không hợp lệ / validate fail → lỗi rõ kèm đường dẫn file.
- Ghi yaml giữ thứ tự key ổn định (sort theo tên) để diff được.

### Keystore — backend `enc` là mặc định

```go
type KeyStore interface {
    Get(name string) (string, error)
    Set(name, secret string) error
}
```

- File `$HI_LLM_HOME/keys.enc`: plaintext là JSON `{ "<name>": "<secret>", ... }`, mã hoá **AES-256-GCM**.
- Format file (version 1):
  `magic "HLLM" (4 byte) | version 0x01 (1 byte) | salt (16) | nonce (12) | ciphertext+tag`.
- Khoá: `argon2.IDKey(password, salt, time=1, memory=64*1024 KiB, threads=4, keyLen=32)`
  (`golang.org/x/crypto/argon2`). Tham số gắn cố định với version 1; đổi tham số = tăng version.
- Mỗi lần ghi tạo **salt và nonce mới** (`crypto/rand`).
- Master password: ưu tiên env `HI_LLM_MASTER_PASSWORD`; không có thì đọc từ terminal ẩn ký tự
  (`golang.org/x/term`), hỏi 2 lần khi tạo `keys.enc` mới (khác nhau → lỗi); không có TTY → lỗi.
- Sai password / magic sai / version lạ / file hỏng → lỗi `master password sai hoặc keys.enc hỏng`,
  **không ghi đè file**.
- Ghi key mới: đọc file cũ (nếu có) → giải mã → thêm/cập nhật entry → mã hoá lại toàn bộ → ghi atomic.
- `Get` tên không có trong file → lỗi `không có key '<name>' trong keys.enc`.
- Backend `env:<VAR>`: đọc `os.Getenv`; rỗng → lỗi `biến môi trường <VAR> chưa đặt`. `Set` → lỗi (read-only).
- Backend `plain:<name>` và `keychain:<name>`: stub, mọi method trả lỗi `chưa hỗ trợ ở phase 1`.
- Hàm resolve con trỏ `Resolve(pointer string) (string, error)` chọn backend theo tiền tố.

### 1. `hi-llm add`

```
hi-llm add [--name N] [--base-url URL] [--kind openai]
           [--api-key-stdin | --api-key-env VAR] [--force] [--skip-verify]
```

- Có TTY và thiếu giá trị: hỏi lần lượt name, base_url, cách lưu key (dán key → `enc`, hoặc tên env var → `env`),
  dán apikey (ẩn ký tự). Không có TTY: thiếu giá trị → lỗi (xem Quy tắc chung).
- `--api-key-stdin`: đọc key từ stdin (dòng đầu, trim) → lưu `enc:<name>`.
- `--api-key-env VAR`: lưu con trỏ `env:VAR`, không ghi keystore.
- Validate:
  - name đúng regex; trùng provider đã có → có TTY thì hỏi ghi đè, không TTY thì cần `--force`, thiếu → lỗi.
  - base_url là URL `http`/`https` có host; path rỗng hoặc `/` → tự nối `/v1`; bỏ `/` cuối.
- Xác minh: `GET {base_url}/models` với `Authorization: Bearer <key>`, kỳ vọng HTTP 200.
  Fail → in cảnh báo (status/lỗi, không in key); có TTY thì hỏi lưu luôn không; không TTY thì exit ≠ 0,
  **không ghi gì**. `--skip-verify` bỏ qua bước này.
- Thứ tự ghi: keystore trước (nếu `enc`), rồi `providers.yaml`.

### 2. `hi-llm ls`

- Bảng: `NAME | KIND | BASE_URL | KEY | MODELS | DEFAULT`.
  - `KEY`: loại con trỏ + trạng thái:
    - `env:FOO ✅` / `env:FOO ❌ thiếu`.
    - `enc`: **`ls` không bao giờ hỏi password**. Có `HI_LLM_MASTER_PASSWORD` → giải mã và hiện `enc ✅` /
      `enc ❌` (không có key hoặc sai password); không có env → `enc 🔒`.
    - `plain`/`keychain`: `<loại> ❌ chưa hỗ trợ`.
  - `MODELS`: số model theo cache + tuổi cache (vd `42 (5m)`); chưa có cache → `-`.
  - `DEFAULT`: tên các mục `defaults` có giá trị bắt đầu bằng `<NAME>/` (vd `chat`), không có → trống.
- Liệt kê cả `subscriptions`: KIND=`sub`, BASE_URL là tên harness, KEY và MODELS là `-`.
- Sắp theo tên. Registry rỗng → in hướng dẫn `hi-llm add`, exit 0.

### 3. `hi-llm models [provider] [--refresh] [--ttl DURATION] [--json]`

- Có `provider`: dùng cache `$HI_LLM_CACHE_DIR/<provider>.json` nếu còn hạn (TTL mặc định `1h`,
  `--ttl` nhận `time.ParseDuration`); hết hạn hoặc `--refresh` → gọi `GET {base_url}/models` (Bearer token)
  rồi ghi cache. Cache lưu `fetched_at` + danh sách model.
- Parse response chuẩn OpenAI: `{"data":[{"id":"...", ...}]}`; thiếu `data` → lỗi rõ.
- Không `provider`: gộp models của mọi provider trong registry **chỉ từ cache** (không gọi mạng, bỏ qua hạn TTL);
  bỏ qua subscriptions; provider chưa có cache → cảnh báo stderr.
- Output mặc định: `provider/model-id` mỗi dòng, sắp xếp (format dùng ngay cho `opencode run -m`);
  `--json` in mảng `[{"provider":..., "id":...}]`.
- Provider không tồn tại hoặc là subscription → lỗi. Gọi mạng lỗi (timeout, 401...) → cảnh báo stderr
  kèm status, exit ≠ 0 (khi chỉ định 1 provider).

### 4. `hi-llm env <provider> [--style openai|anthropic]`

- In ra stdout các dòng để `eval "$(hi-llm env deeprelay)"`:
  - `--style openai` (mặc định): `OPENAI_API_KEY`, `OPENAI_BASE_URL` (= `base_url`).
  - `--style anthropic`: `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_BASE_URL` (= `base_url` **bỏ `/v1` ở cuối**,
    vì SDK Anthropic tự nối `/v1/messages`).
- Mọi giá trị đặt trong nháy đơn, nháy đơn bên trong thoát thành `'\''`:
  `export OPENAI_API_KEY='sk-...'`.
- Secret resolve từ keystore lúc chạy; đây là lệnh duy nhất được in secret (ra stdout).
- Provider subscription → lỗi `provider này là subscription CLI, không có env`. Style lạ → lỗi.

## Cấu trúc code

```
hi-tools/                        # gốc repo — go.mod chung (module hi-tools), Makefile chung đã có sẵn
├── cmd/hi-llm/main.go           # entrypoint: run(args, stdin, stdout, stderr), dispatch subcommand
├── internal/
│   ├── llm/
│   │   ├── registry/            # load/save/validate providers.yaml
│   │   ├── keystore/            # KeyStore interface, encStore, envStore, stub plain/keychain, Resolve
│   │   └── provider/            # client GET /models, parse response, cache JSON + TTL
│   └── hix/                     # hạ tầng CLI dùng chung — đã có doc.go
└── hi-llm/README.md             # cập nhật usage Phase 1
```

`internal/hix` (Phase 1 chỉ gồm):
- `IsTTY(r io.Reader) bool` — true nếu là `*os.File` gắn terminal.
- `ReadPassword(prompt string, stdin io.Reader, stderr io.Writer) (string, error)` — ẩn ký tự qua `x/term`;
  không TTY → lỗi.
- `Table` — in bảng căn cột (`text/tabwriter`), header + rows.

Import path dạng `hi-tools/internal/llm/registry`, `hi-tools/internal/hix`.

- CLI parsing: stdlib `flag` (`flag.NewFlagSet` cho mỗi subcommand). Không dùng cobra.
- YAML: `gopkg.in/yaml.v3`. Crypto: `golang.org/x/crypto/argon2`, `crypto/aes`, `crypto/cipher`. Terminal: `golang.org/x/term`.

## Tiêu chí hoàn thành

- End-to-end trên máy thật (người kiểm ở gate 2): `hi-llm add` (provider thật, vd deeprelay) →
  `HI_LLM_MASTER_PASSWORD=... hi-llm ls` hiện `enc ✅` → `hi-llm models deeprelay` ra danh sách →
  `eval "$(hi-llm env deeprelay)"` và curl thử thành công.
- `keys.enc` mở bằng text editor chỉ thấy binary rác; `providers.yaml` không chứa secret.
- Unit test (không network thật — dùng `httptest`; không đụng `~` — dùng `t.TempDir()` + các env `HI_LLM_*`):
  vòng đời encStore (tạo / sai password không ghi đè / thêm key / file hỏng / salt-nonce mới mỗi lần ghi),
  parse + validate registry (tên sai regex, kind lạ, con trỏ lạ), resolve `env:`, cache TTL + `--refresh`,
  parse response `/models`, `add` không TTY (thiếu giá trị → lỗi, trùng tên cần `--force`, verify fail không ghi),
  `ls` không lộ secret, output `env` cả 2 style (quoting, bỏ `/v1` cho anthropic).
- `go vet` sạch, `go test ./...` pass.

verify: go build ./... && go vet ./... && go test ./...

## Tasks gợi ý (1-5 task, đúng ràng buộc của flow)

- [ ] `internal/hix` (IsTTY, ReadPassword, Table) + skeleton CLI `cmd/hi-llm` (`run(...)`, dispatch subcommand bằng `flag`) + package `registry`: load/save/validate `providers.yaml`
- [ ] Package `keystore`: interface + `encStore` (format HLLM v1, argon2id + AES-GCM, ghi atomic 0600, không ghi đè khi sai password) + `envStore` + stub plain/keychain + `Resolve`
- [ ] Package `provider` (client `GET /models`, parse, cache JSON TTL) + lệnh `add` (TTY wizard + flags, validate, verify, `--force`, `--skip-verify`)
- [ ] Lệnh `models` (`--refresh`, `--ttl`, `--json`, gộp từ cache) và lệnh `ls` (bảng trạng thái, không hỏi password, không lộ secret)
- [ ] Lệnh `env` (`--style openai|anthropic`, quoting, bỏ `/v1` cho anthropic) + cập nhật `hi-llm/README.md` phần usage Phase 1

## Ngoài phạm vi (để phase sau)

`ping` health check, `use` với fzf, harness adapter (`sync opencode`, `sync commandcode`), `role` cho my-flow,
backend `plain`/`keychain` hoàn chỉnh, cache master password trong session, cảnh báo quota,
format API không phải OpenAI-compatible, lệnh xoá/sửa provider.
