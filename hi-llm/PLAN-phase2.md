verify: go build ./... && go vet ./... && go test ./...

## API

Code Phase 1 đã có — chỉ thêm/sửa đúng các mục dưới, giữ nguyên chữ ký hàm đang có (test cũ phải pass không sửa).

- internal/llm/registry/registry.go:
  - `type Provider struct { Kind; BaseURL; APIKey; Headers map[string]string \`yaml:"headers,omitempty"\`; Harnesses []string \`yaml:"harnesses,omitempty"\` }`
  - `func (p Provider) HarnessList() []string` — `Harnesses` rỗng → `[]string{"opencode"}`.
  - `Validate` thêm: mỗi phần tử `Harnesses` phải qua `ValidName` → lỗi `provider "<tên>": harness "<h>" không hợp lệ`.
  - `type Subscription struct { Harness string \`yaml:"harness"\`; Models []string \`yaml:"models,omitempty"\` }`
  - `Validate` thêm: tên header rỗng / có khoảng trắng hoặc `:` / bằng `Authorization` (không phân biệt hoa thường) → lỗi `provider "<tên>": header "<h>" không hợp lệ`.
- internal/llm/provider/provider.go:
  - `type Client struct { HTTP *http.Client; Headers map[string]string }` — `ListModels` và `Chat` set từng header trong `Headers` (sau `Authorization`).
  - `func (c *Client) Chat(ctx context.Context, baseURL, apiKey, model string) error` — `POST {baseURL}/chat/completions`, `Content-Type: application/json`, body `{"model":model,"messages":[{"role":"user","content":"ping"}],"max_tokens":16}`; status ≠ 200 → `*HTTPError`; không đọc nội dung.
  - `func Classify(err error) string` — `*HTTPError` 401/403 → `auth`, 402/429 → `quota`, khác → `http`; lỗi có `net.Error` hoặc `*url.Error` trong chuỗi bọc → `net`; còn lại → `response`. (`key` do lệnh ping tự gán.)
- internal/llm/provider/ping.go:
  - `type PingResult struct { At time.Time \`json:"at"\`; OK bool \`json:"ok"\`; LatencyMS int64 \`json:"latency_ms"\`; Kind string \`json:"kind,omitempty"\`; Error string \`json:"error,omitempty"\` }`
  - `func LoadPings(dir string) map[string]PingResult` — đọc `<dir>/_ping.json`; thiếu/hỏng → map rỗng (không lỗi).
  - `func SavePings(dir string, results map[string]PingResult) error` — đọc file cũ, ghi đè các key trong `results`, giữ key khác, `hix.WriteFileAtomic` 0600.
- internal/llm/keystore/enc.go:
  - `func OncePassword(pw PasswordFunc) PasswordFunc` — gọi `pw` lần đầu thành công rồi trả lại giá trị đó cho các lần sau (an toàn khi gọi từ một goroutine; ping resolve key tuần tự). Lỗi thì không nhớ.
- cmd/hi-llm/ping.go: `func runPing(args []string, stdin io.Reader, stdout, stderr io.Writer) int`
  - Resolve key tuần tự bằng `keystore.Resolver{Enc: keystore.NewEncStore(path, keystore.OncePassword(keystore.PromptPassword(stdin, stderr)))}`; lỗi → kết quả `{OK:false, Kind:"key"}` không gửi request.
  - Ping song song (`sync.WaitGroup`), mỗi provider một `provider.Client` có `Headers`; chế độ mặc định dùng `ListModels` rồi `SaveCache`; `--chat` dùng `Chat`.
  - In bảng `hix.Table` `NAME STATUS LATENCY DETAIL` theo tên; `SavePings`; exit 0/1 theo spec.
- cmd/hi-llm/ls.go: thêm cột `PING` từ `provider.LoadPings(cacheDir)` — `func pingStatus(r provider.PingResult, ok bool, now time.Time) string`.
- cmd/hi-llm/add.go: provider đã có → `Headers` cũ được giữ trong bản ghi mới và gán vào client verify.
- cmd/hi-llm/models.go: client có `Headers` của provider.
- cmd/hi-llm/role.go: `func runRole(args []string, stdin io.Reader, stdout, stderr io.Writer) int`
  - `func rolesPath() (string, error)` — `HI_LLM_ROLES` → `exec.LookPath("flow")` + `filepath.EvalSymlinks` + `roles.conf` → lỗi.
  - `func roleCandidates(reg *registry.Registry, cacheDir string) []string` — provider: `<h>:<provider>/<model>` cho mỗi `h` trong `HarnessList()`; subscription: `<harness>:<model>`; sort, bỏ trùng.
  - `func readRoles(data []byte) (order []string, values map[string]string)`.
  - `func setRole(data []byte, role, value string) ([]byte, bool)` — thay mọi dòng `^role=`, bool = có dòng của role; giữ nguyên phần còn lại byte-for-byte.
  - Biến gói `var pickFn = fzfPick` (`func(prompt string, cands []string) (string, error)`) để test thay được; `fzfPick` chạy `fzf --prompt`, stdin = candidates nối `\n`, `Stderr = os.Stderr`; exit 1/130 → lỗi huỷ.
- cmd/hi-llm/main.go: dispatch `ping`, `role`; usage thêm 2 dòng.

## Test

| AC | File test | Symbol |
|---|---|---|
| Header + harnesses: yaml có `headers`/`harnesses` load/save được; validate cấm `Authorization`/tên rỗng/có `:`, harness sai tên; `HarnessList` mặc định `[opencode]` | internal/llm/registry/registry_test.go | `Parse`, `Registry.Validate` |
| Header gửi kèm `/models` và chat; `Chat` gửi đúng method/path/body, 200 → nil, 429 → `*HTTPError`; `Classify` đủ auth/quota/http/net/response | internal/llm/provider/ping_test.go | `Client.ListModels`, `Client.Chat`, `Classify` |
| `_ping.json`: thiếu/hỏng → rỗng; `SavePings` gộp giữ key cũ | internal/llm/provider/ping_test.go | `LoadPings`, `SavePings` |
| `OncePassword` gọi hàm gốc đúng 1 lần khi thành công, gọi lại sau lỗi | internal/llm/keystore/enc_test.go | `OncePassword` |
| `ping` 2 provider (1 ok, 1 trả 401): bảng đúng thứ tự, exit 1, `_ping.json` có cả hai, cache model của provider ok được ghi; provider `env:` chưa đặt → ❌ `key`, server không nhận request; tên lạ / subscription → exit 2; `--chat` với 0 hoặc 2 provider → exit 2; `--chat` 1 provider gửi đúng model; header của provider tới server; registry rỗng → exit 0 | cmd/hi-llm/ping_test.go | `run` (`ping`) |
| `add --force` giữ `headers` cũ và verify gửi header đó | cmd/hi-llm/add_test.go | `run` (`add`) |
| `ls` cột PING: có `_ping.json` → `✅ <n>ms (<tuổi>)` / `❌ <kind> (<tuổi>)`; không có → `-` | cmd/hi-llm/ls_test.go | `run` (`ls`), `pingStatus` |
| `role`: bảng hiện tại (role trùng lấy dòng cuối, bỏ comment); đặt value trong candidates → file đổi đúng dòng, giữ comment/thứ tự/newline cuối/quyền; value không đổi → không ghi; role không có → exit 2; value ngoài candidates → exit 2, `--force` thì ghi; `--force` sai dạng `harness:model` → exit 2; không value + không TTY → exit 2; `pickFn` giả trả lựa chọn / lỗi huỷ; candidates từ cache provider (mặc định `opencode:`, `harnesses: [opencode, commandcode]` → cả hai) + `models` của subscription; `rolesPath` theo `HI_LLM_ROLES` | cmd/hi-llm/role_test.go | `run` (`role`), `roleCandidates`, `readRoles`, `setRole`, `rolesPath` |

## Task
- [ ] 1. Header riêng của provider + nền cho ping: `Provider.Headers`, `Provider.Harnesses` + `HarnessList` (+ validate), `Subscription.Models`, `Client.Headers` áp vào `ListModels`, `Client.Chat`, `Classify`, `keystore.OncePassword`; `models` và verify của `add` dùng header, `add` ghi đè giữ header cũ (internal/llm/registry/registry.go, internal/llm/provider/provider.go, internal/llm/keystore/enc.go, cmd/hi-llm/models.go, cmd/hi-llm/add.go)
- [ ] 2. Lệnh `ping`: lưu kết quả `_ping.json` (`PingResult`, `LoadPings`, `SavePings` gộp), resolve key tuần tự một lần hỏi password, ping song song, `--chat`, bảng + exit code, ghi cache model khi ok; dispatch + usage (internal/llm/provider/ping.go, cmd/hi-llm/ping.go, cmd/hi-llm/main.go)
- [ ] 3. Cột `PING` trong `ls` đọc `_ping.json` (cmd/hi-llm/ls.go)
- [ ] 4. Lệnh `role`: tìm `roles.conf`, bảng hiện tại, candidates (cache provider + `models` của subscription), đặt trực tiếp / `--force` / fzf qua `pickFn`, ghi giữ nguyên phần còn lại và quyền file; dispatch + usage (cmd/hi-llm/role.go, cmd/hi-llm/main.go)
- [ ] 5. Docs: `hi-llm/README.md` (bảng lệnh + lộ trình: Phase 2 = ping + headers + role, bỏ `use`/`defaults`), `guides/hi-llm/commands.md` (ping, role, headers, cột PING), `guides/hi-llm/troubleshooting.md` (bỏ dòng "Chưa có: ping..." cho phần đã làm, thêm lỗi của ping/role) (hi-llm/README.md, guides/hi-llm/commands.md, guides/hi-llm/troubleshooting.md)
