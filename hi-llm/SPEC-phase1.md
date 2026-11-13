# SPEC: hi-llm Phase 1 — registry provider + keystore mã hoá + add/ls/models/env

## Bối cảnh

`hi-llm` là CLI quản lý tập trung nhiều LLM provider và sinh config cho nhiều harness
(xem README.md). Phase 1 xây phần lõi: registry `providers.yaml`, keystore mã hoá
(`keys.enc` + master password), và 4 lệnh `add`, `ls`, `models`, `env`.

Chưa làm trong Phase 1: `ping`, `use` (fzf), harness adapter (`sync`, `role`), health check định kỳ.

Ngôn ngữ: **Go**. Repo: `hi-tools` — monorepo, **module Go chung ở gốc** (`go.mod` đã có sẵn, module `hi-tools`).
Code của tool này nằm ở `cmd/hi-llm/` + `internal/llm/`; hạ tầng CLI dùng chung với các tool
khác (bảng kết quả có màu, đọc password ẩn ký tự, fzf...) đặt ở `internal/hix/` — phần nào
dùng chung được thì viết vào `hix` thay vì duplicating.
Chạy được trên macOS và Linux (không cgo, không phụ thuộc OS keystore ở đường mặc định).

## Phạm vi Phase 1

### Registry — `~/.config/hi-llm/providers.yaml`

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

- Thư mục config override bằng env `HI_LLM_HOME` (mặc định `~/.config/hi-llm/`).
- File yaml không hợp lệ / thiếu → báo lỗi rõ kèm đường dẫn file, không panic.
- Ghi yaml giữ thứ tự key ổn định (sort theo tên) để diff được.

### Keystore — backend `enc` là mặc định

- File `~/.config/hi-llm/keys.enc`: JSON `{ "<name>": "<secret>", ... }` mã hoá **AES-256-GCM**;
  khoá dẫn xuất từ master password bằng **argon2id** (`golang.org/x/crypto/argon2`);
  file format: `magic | version | salt(16) | nonce(12) | ciphertext`.
- Master password: đọc từ terminal ẩn ký tự (`golang.org/x/term`), hỏi 2 lần khi tạo mới;
  override không interactive bằng env `HI_LLM_MASTER_PASSWORD` (cho script/test).
- Sai password / file hỏng → lỗi rõ ràng "master password sai hoặc keys.enc hỏng", **không ghi đè**.
- Ghi key mới: đọc file cũ (nếu có) → giải mã → thêm/cập nhật entry → mã hoá lại toàn bộ → ghi atomic (tmp + rename).
- Backend `env:<VAR>`: đọc thẳng `os.Getenv`.
- Backend `plain:<name>` và `keychain:<name>`: định nghĩa interface + stub trả lỗi "chưa hỗ trợ ở phase 1" (interface sẵn để phase sau cài).

```go
type KeyStore interface {
    Get(name string) (string, error)
    Set(name, secret string) error
}
```

### 1. `hi-llm add`

Wizard (hoặc flags cho không interactive):

```
hi-llm add --name deeprelay --base-url https://.../v1 [--api-key-stdin] [--skip-verify]
```

- Không flags: hỏi lần lượt name, base_url, dán apikey (ẩn ký tự).
- Validate: name duy nhất (trùng thì hỏi ghi đè), base_url là URL hợp lệ, tự nối `/v1` nếu thiếu path.
- Lưu key: hỏi master password → `enc:<name>` (mặc định); nếu user chọn "dùng env var có sẵn" → nhập tên biến → `env:<VAR>`.
- Xác minh: gọi `GET {base_url}/models` với `Authorization: Bearer <key>`; fail → cảnh báo nhưng cho phép `--skip-verify` / xác nhận lưu anyway.
- Ghi vào `providers.yaml`.

### 2. `hi-llm ls`

- Bảng: `NAME | KIND | BASE_URL | KEY | MODELS | DEFAULT`.
  - `KEY`: loại con trỏ + trạng thái resolve được hay không (`enc ✅`, `env:FOO ❌ thiếu`).
  - `MODELS`: số model theo cache (chưa có cache → `-`), kèm tuổi cache.
  - `DEFAULT`: đánh dấu nếu provider/model nằm trong `defaults`.
- Liệt kê cả mục `subscriptions` (kind=`sub`, harness tương ứng).
- **Không in secret** ra terminal dưới mọi hình thức.

### 3. `hi-llm models [provider]`

- Có `provider`: gọi `GET {base_url}/models` (Bearer token), cache vào `~/.cache/hi-llm/<provider>.json` TTL mặc định 1h (`--refresh` bỏ qua cache, `--ttl` override).
- Không `provider`: gộp models của mọi provider trong registry (chỉ dùng cache, không gọi mạng hàng loạt).
- Output mặc định: `provider/model-id` mỗi dòng (format dùng được ngay cho `opencode run -m`); `--json` in JSON đầy đủ.
- Provider relay lỗi (timeout, 401): in cảnh báo stderr, các provider khác vẫn chạy.

### 4. `hi-llm env <provider> [--style openai|anthropic]`

- In ra stdout các dòng `export KEY=VALUE` để `eval $(hi-llm env deeprelay)`:
  - `--style openai` (mặc định): `OPENAI_API_KEY`, `OPENAI_BASE_URL`.
  - `--style anthropic`: `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_BASE_URL`.
- Giá trị secret resolve từ keystore lúc chạy; chỉ lệnh này được in secret (ra stdout để eval).
- Provider subscription → lỗi "provider này là subscription CLI, không có env".

## Cấu trúc code

```
hi-tools/                        # gốc repo — go.mod chung (module hi-tools), Makefile chung đã có sẵn
├── cmd/hi-llm/main.go           # entrypoint, dispatch subcommand (đã có placeholder)
├── internal/
│   ├── llm/
│   │   ├── registry/            # load/save providers.yaml, validate, resolve con trỏ key
│   │   ├── keystore/            # KeyStore interface + encStore (argon2id+AES-GCM) + envStore + stub
│   │   └── provider/            # client /models (stdlib net/http), cache JSON + TTL
│   └── hix/                     # hạ tầng CLI dùng chung — đã có doc.go
└── hi-llm/README.md             # cập nhật usage Phase 1
```

Import path dạng `hi-tools/internal/llm/registry`, `hi-tools/internal/hix`.

- CLI parsing: stdlib `flag` + subcommand dispatch (đồng bộ lựa chọn với hi-brains; nếu hi-brains dùng cobra thì theo cobra).
- YAML: `gopkg.in/yaml.v3`. Crypto: `golang.org/x/crypto/argon2`, `golang.org/x/term`. HTTP: stdlib.
- Không cgo, không network ngoài `GET /models`.

## Tiêu chí hoàn thành

- End-to-end trên máy thật: `hi-llm add` (provider thật, vd deeprelay) → `hi-llm ls` hiện ✅ → `hi-llm models deeprelay` ra danh sách → `eval $(hi-llm env deeprelay)` và curl thử thành công.
- `keys.enc` mở bằng text editor chỉ thấy binary rác; `providers.yaml` không chứa secret.
- Unit test phủ: vòng đời encStore (tạo/sai password/thêm key/ghi atomic), parse+validate registry, resolve `env:`, cache TTL, parse response `/models`, sinh output `env` cả 2 style.
- `go vet` sạch, `go test ./...` pass.

verify: go build ./... && go vet ./... && go test ./...

## Tasks gợi ý (1-5 task, đúng ràng buộc của flow)

- [ ] Skeleton CLI trong `cmd/hi-llm` (subcommand dispatch; go.mod + Makefile đã có sẵn ở gốc repo) + package `registry`: load/save/validate `providers.yaml`, con trỏ key
- [ ] Package `keystore`: interface + `encStore` (master password qua x/term hoặc env, argon2id + AES-GCM, ghi atomic, không ghi đè khi sai password) + `envStore` + stub plain/keychain
- [ ] Lệnh `add`: wizard + flags không interactive, validate name/base_url, lưu key, gọi thử `GET /models` xác minh (có `--skip-verify`)
- [ ] Package `provider` + lệnh `models` (cache JSON TTL 1h, `--refresh`, `--json`, gộp mọi provider) và lệnh `ls` (bảng trạng thái, không lộ secret)
- [ ] Lệnh `env` (`--style openai|anthropic`, resolve keystore lúc chạy) + unit test các package + cập nhật README (Makefile chung đã có sẵn ở gốc)

## Ngoài phạm vi (để phase sau)

`ping` health check, `use` với fzf, harness adapter (`sync opencode`, `sync commandcode`), `role` cho my-flow, backend `plain`/`keychain` hoàn chỉnh, cảnh báo quota, format API không phải OpenAI-compatible.
