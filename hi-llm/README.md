# hi-llm

Quản lý tập trung nhiều LLM provider (API relay + subscription CLI) và sinh config cho nhiều harness (opencode, commandcode, my-flow...). Một registry duy nhất — hết cảnh copy-paste apikey/baseurl/model vào từng chỗ.

## Vấn đề

- Nhiều provider OpenAI-compatible (apikey + base_url): `opencode-go`, `deeprelay`, ... — mỗi nơi tiêu thụ (opencode, commandcode, env, my-flow `roles.conf`) lại phải cấu hình tay một lần.
- Ngoài ra có các subscription CLI đã login sẵn: `claude` (Claude Code), `agy` (Gemini CLI) — cần liệt kê chung một chỗ để chọn.
- Đổi model/provider thường xuyên (theo giá, quota, chất lượng) → cần công cụ đổi nhanh, không sửa tay config.

## Mô hình

Hai loại nguồn model:

| Loại | Ví dụ | Auth | List models |
|---|---|---|---|
| API relay (OpenAI-compatible) | opencode-go, deeprelay | apikey + base_url | `GET {base_url}/models` |
| Subscription CLI | claude, agy | đã login trong CLI | hỏi chính CLI |

Và nhiều nơi tiêu thụ: `~/.config/opencode/opencode.json`, config commandcode, `my-flow/roles.conf`, env vars.

→ **Registry là nguồn sự thật duy nhất; config các harness là bản chiếu được sinh ra.**

## Registry

`~/.config/hi-llm/providers.yaml`:

```yaml
providers:
  deeprelay:
    kind: openai
    base_url: https://.../v1
    api_key: enc:deeprelay        # con trỏ tới keystore
  opencode-go:
    kind: openai
    base_url: https://.../v1
    api_key: env:OPENCODE_API_KEY # hoặc env var có sẵn
subscriptions:
  claude: { harness: claude }
  agy:    { harness: agy }
defaults:
  chat: opencode-go/qwen3.8-max
  review: claude/claude-opus-5-5
```

## Lưu API key — backend `enc` (mặc định)

- File `~/.config/hi-llm/keys.enc`: AES-GCM, khoá dẫn xuất từ **master password** bằng argon2id — thuần Go, không cgo, chạy được cả macOS lẫn Ubuntu/VPS.
- Master password hỏi qua terminal (ẩn ký tự, `golang.org/x/term`), cache trong session để không hỏi lại mỗi lệnh.
- Sai password → giải mã fail → báo lỗi rõ, không ghi đè.
- `providers.yaml` chỉ chứa *con trỏ*, copy cả thư mục config sang máy khác là dùng được.

Các backend phụ (hỗ trợ, không mặc định):

| Con trỏ | Ý nghĩa |
|---|---|
| `enc:<name>` | keys.enc + master password (mặc định, portable) |
| `env:<VAR>` | đọc từ environment variable |
| `plain:<name>` | file `keys.json` chmod 600 |
| `keychain:<name>` | macOS Keychain (gọi `security` qua os/exec) |

## Các lệnh

| Phase | Lệnh | Chức năng |
|---|---|---|
| 1 | `hi-llm add` | wizard thêm provider: tên → base_url → dán key → master password → gọi thử `/v1/models` xác nhận → ghi registry |
| 1 | `hi-llm ls` | bảng tổng quan: provider, loại, số model (cache), key hợp lệ?, ping lần cuối |
| 1 | `hi-llm models [provider]` | gọi `{base_url}/models`, cache TTL, in bảng hoặc mở fzf |
| 1 | `hi-llm env <provider>` | in `export ...` (OPENAI_API_KEY/BASE_URL hoặc ANTHROPIC_*) → `eval $(hi-llm env deeprelay)` |
| 2 | `hi-llm ping [provider\|all]` | health check mỗi provider (request tí hon), ✅/❌ + độ trễ + lỗi quota/auth |
| 2 | `hi-llm use` | fzf 2 bước: chọn provider → chọn model → ghi vào nơi tiêu thụ được chọn |
| 3 | `hi-llm sync [harness]` | sinh config harness từ registry (không tham số = tất cả) |
| 3 | `hi-llm role <role> [provider/model]` | cập nhật dòng role trong my-flow `roles.conf` |

## Harness adapter (Phase 3)

```go
type Harness interface {
    Name() string
    Sync(providers []Provider) error   // ghi config của harness
    SetRole(target, model string) error // optional (my-flow dùng)
}
```

- `harness_opencode.go` → ghi `~/.config/opencode/opencode.json`
- `harness_commandcode.go` → config commandcode
- Thêm harness mới = viết 1 file adapter; registry/các lệnh khác không đổi.
- Subscription CLI không cần sync — chỉ được liệt kê trong `use`/`role`.

## Kịch bản dùng hàng ngày

```sh
hi-llm ping                       # sáng: bên nào sống, bên nào hết quota
hi-llm role implement             # fzf chọn model cho flow role
hi-llm role review                # role quan trọng → model mạnh
flow new my-feature "..."         # my-flow đọc roles.conf

hi-llm add && hi-llm sync         # thêm provider mới → mọi harness dùng được ngay
eval $(hi-llm env deeprelay)      # script/CLI bất kỳ dùng provider này
```

## Cấu trúc repo (monorepo `hi-tools` — module Go chung ở gốc)

```
hi-tools/
├── go.mod                    # module hi-tools
├── Makefile                  # make build/install (mọi binary trong cmd/)
├── cmd/hi-llm/               # main.go, dispatch subcommand
├── internal/
│   ├── llm/
│   │   ├── registry/         # providers.yaml, resolve con trỏ key
│   │   ├── keystore/         # interface + backend enc (mặc định), env, plain, keychain
│   │   ├── provider/         # client /v1/models, ping, cache TTL
│   │   └── harness/          # interface + adapter opencode, commandcode (phase 3)
│   └── hix/                  # hạ tầng CLI dùng chung giữa các tool
└── hi-llm/                   # docs của tool này
    ├── README.md
    └── SPEC-phase1.md        # spec Phase 1 (đầu vào cho my-flow)
```

## Lộ trình

| Phase | Nội dung |
|---|---|
| 1 | Registry + keystore `enc` + `add`/`ls`/`models`/`env` ← đang làm |
| 2 | `ping` + cache + `use` (fzf) |
| 3 | Harness adapter: `sync opencode`, `sync commandcode`, `role` cho my-flow |
| 4 | (tuỳ hứng) health check định kỳ, cảnh báo quota |

Phạm vi relay hiện tại: chỉ OpenAI-compatible `/v1/models`; format khác tính sau.

## Build & install

Chạy từ gốc repo `hi-tools/` (Makefile chung):

```sh
make build           # build mọi binary vào ./bin
make install         # copy vào $GOBIN
make build hi-llm    # chỉ build tool này
```
