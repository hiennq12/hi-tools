# hi-llm — bắt đầu từng bước

`hi-llm` giữ **một chỗ duy nhất** cho mọi LLM provider (base_url + API key) để không phải copy key vào
từng tool. Phase 1 có 4 lệnh: `add`, `ls`, `models`, `env`.

- Tra cứu từng lệnh: [commands.md](commands.md)
- Gặp lỗi: [troubleshooting.md](troubleshooting.md)

---

## 0. Ba khái niệm cần nhớ

**Provider vs subscription**

| | provider | subscription |
|---|---|---|
| Là gì | API OpenAI-compatible (có key + base_url, gọi được bằng `curl`) | CLI đã login sẵn (`claude`, `agy`, `commandcode`...) |
| Ví dụ | deeprelay, opencode-go | claude |
| Thêm bằng | `hi-llm add` | sửa tay `providers.yaml` (bước 6) |

**Con trỏ key** — `providers.yaml` không chứa key, chỉ chứa *chỗ để lấy key*:

| Con trỏ | Key thật nằm ở |
|---|---|
| `enc:deeprelay` | file `keys.enc` (mã hoá AES-256-GCM bằng **master password**), mục `deeprelay` |
| `env:OPENCODE_API_KEY` | biến môi trường `$OPENCODE_API_KEY` |

**File nằm ở đâu**

| File | Mặc định | Đổi bằng env |
|---|---|---|
| `providers.yaml` (registry, không có secret) | `~/.config/hi-llm/` | `HI_LLM_HOME` |
| `keys.enc` (key mã hoá, quyền 600) | `~/.config/hi-llm/` | `HI_LLM_HOME` |
| `<provider>.json` (cache danh sách model) | `~/.cache/hi-llm/` | `HI_LLM_CACHE_DIR` |

> ⚠️ **Master password không khôi phục được.** Quên = phải xoá `keys.enc` và `add` lại các key.
> Ghi nó vào password manager.

---

## 1. Build & cài

```sh
cd ~/go/src/my-project/my-tool/hi-tools
make build hi-llm && make install     # → $GOBIN/hi-llm
hi-llm                                # in usage = cài thành công
```

- `make ...` phải chạy ở **gốc repo** `hi-tools/` (nơi có `Makefile`, `go.mod`).
- Sau `make install`, lệnh `hi-llm` chạy được **ở bất kỳ thư mục nào** (`~/go/bin` có trong PATH);
  tool không phụ thuộc thư mục hiện tại — luôn đọc `~/.config/hi-llm/` và `~/.cache/hi-llm/`.
- Sửa code xong → chạy lại `make install` để cập nhật binary.

## 2. Thêm provider đầu tiên

Gõ `hi-llm add` trong terminal, wizard hỏi lần lượt:

```
Tên provider: deeprelay                       ← chỉ a-z 0-9 . _ - , bắt đầu bằng chữ/số
Base URL: https://relay.example.com           ← thiếu path thì tự nối /v1
Cách lưu key — (1) enc, (2) env: 1            ← 1 = mã hoá vào keys.enc (khuyên dùng)
API key: ********                             ← dán key, không hiện ký tự
Master password: ********                     ← lần đầu: đặt password mới
Nhập lại master password: ********            ←   (chỉ hỏi lần tạo keys.enc)
đã thêm provider deeprelay (https://relay.example.com/v1)
```

- Trước khi lưu, hi-llm gọi thử `GET {base_url}/models` bằng key vừa nhập. Sai key / sai URL →
  `cảnh báo: xác minh ... thất bại` + hỏi `Vẫn lưu provider? [y/N]`.
- Chọn `2` (env) nếu key đã có sẵn trong biến môi trường → hỏi `Tên biến môi trường:` → lưu `env:<VAR>`,
  không đụng `keys.enc`, không hỏi master password.
- Tên đã tồn tại → hỏi `Provider "x" đã tồn tại, ghi đè? [y/N]`.

## 3. Xem tổng quan — `hi-llm ls`

```
$ HI_LLM_MASTER_PASSWORD='...' hi-llm ls
NAME       KIND    BASE_URL                         KEY                MODELS
claude     sub     claude                           -                  -
deeprelay  openai  https://relay.example.com/v1     enc ✅              3 (5m)
opencode   openai  https://opencode.example.com/v1  env:OC_KEY ❌ thiếu  -
```

- `ls` **không bao giờ hỏi password**. Không đặt `HI_LLM_MASTER_PASSWORD` → cột KEY hiện `enc 🔒` (chưa kiểm).
- `MODELS`: số model trong cache + tuổi cache; `-` = chưa từng chạy `models <provider>`.

## 4. Lấy danh sách model — `hi-llm models`

```
$ hi-llm models deeprelay            # hỏi master password (nếu enc), gọi /models, cache 1h
deeprelay/deepseek-v4.1-flash
deeprelay/glm-5.3
deeprelay/qwen3.8-max

$ hi-llm models                      # gộp mọi provider, CHỈ đọc cache, không gọi mạng
```

Dòng `provider/model-id` dán thẳng được vào `opencode run -m ...` hoặc `my-flow/roles.conf`.

## 5. Dùng key trong shell — `hi-llm env`

```sh
eval "$(hi-llm env deeprelay)"                     # OPENAI_API_KEY + OPENAI_BASE_URL
curl -s "$OPENAI_BASE_URL/models" -H "Authorization: Bearer $OPENAI_API_KEY" | head -c 200

eval "$(hi-llm env deeprelay --style anthropic)"   # ANTHROPIC_AUTH_TOKEN + ANTHROPIC_BASE_URL (bỏ /v1)
```

Luôn có **dấu ngoặc kép** quanh `$(...)`. Lời hỏi password in ra stderr nên vẫn gõ được bình thường.

## 6. Khai báo subscription (sửa tay)

Mở `~/.config/hi-llm/providers.yaml`, thêm:

```yaml
subscriptions:
  claude: { harness: claude }
  agy:    { harness: agy }
```

Tên subscription không được trùng tên provider. Phase 1 chỉ dùng để hiện trong `ls`.

## 7. Dùng trong script / cron (không có terminal)

Không có terminal thì hi-llm **không hỏi gì** — thiếu thông tin là báo lỗi ngay (không treo):

```sh
export HI_LLM_MASTER_PASSWORD='...'                       # thay cho việc gõ password
printf '%s' "$KEY" | hi-llm add --name deeprelay \
  --base-url https://relay.example.com --api-key-stdin     # key qua stdin
hi-llm add --name oc --base-url https://... --api-key-env OC_KEY   # hoặc trỏ env
hi-llm add ... --force          # ghi đè provider đã có
hi-llm add ... --skip-verify    # không gọi thử /models
```

> Không để `HI_LLM_MASTER_PASSWORD` cố định trong `~/.zshrc` — như vậy mã hoá mất tác dụng.
> Chỉ export trong phiên / script cần dùng.

## 8. Backup & chuyển sang máy khác

```sh
tar czf hi-llm-backup.tgz -C ~/.config hi-llm      # providers.yaml + keys.enc
# máy mới: giải nén vào ~/.config/, cài hi-llm, dùng cùng master password
```

`providers.yaml` an toàn để commit/chia sẻ (không có secret). `keys.enc` chỉ mở được bằng master password.
