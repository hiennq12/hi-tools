# SPEC: hi-brains Phase 1 — ghi note nhanh, tìm lại được ngay

## Bối cảnh

`hi-brains` là CLI quản lý tri thức kỹ thuật cá nhân cho lập trình viên (xem README.md).
Phase 1 xây phần lõi: ghi note dạng markdown + frontmatter YAML vào thư mục git, tìm kiếm
nhanh bằng ripgrep/fzf. Chưa làm spaced repetition, digest, kata, import note cũ, RSS.

Ngôn ngữ: **Go**. Repo: `hi-tools/hi-brains` (module Go riêng trong folder này).

## Phạm vi Phase 1

4 lệnh: `init`, `add`, `search`, `list`.

### 1. `hi-brains init`

- Tạo thư mục notes, mặc định `~/brains/`, override bằng env `BRAINS_DIR`.
- Nếu thư mục chưa phải git repo thì `git init`.
- Báo lỗi thân thiện nếu thư mục đã tồn tại và không rỗng (không ghi đè).
- Idempotent: chạy lại không phá dữ liệu.

### 2. `hi-brains add`

```
hi-brains add                                        # hỏi type → mở $EDITOR với template
hi-brains add --type solution                        # mở thẳng template solution
hi-brains add --quick "redis timeout: tăng pool + dial timeout" --tags redis,go
```

- `--type` nhận 1 trong: `solution | adr | concept | kata | snippet`. Template tương ứng:
  - `solution`: mục *Triệu chứng / Nguyên nhân / Cách fix*
  - `adr`: mục *Bối cảnh / Quyết định / Hệ quả*
  - `concept`: mục *Tóm tắt / Chi tiết / Nguồn*
  - `kata`: mục *Đề bài / Approach / Code*
  - `snippet`: mục *Mô tả / Code*
- Không `--type` và không `--quick`: prompt chọn type trong terminal rồi mở editor.
- `--quick "<text>"`: không mở editor; tạo file ngay, title sinh từ nội dung (câu đầu, tối đa ~60 ký tự), body là text.
- `--tags a,b,c`: danh sách tag phân tách dấu phẩy.
- Editor: `$EDITOR`, fallback `vi`. Nếu người dùng xoá hết nội dung → huỷ, không tạo file (như git commit).
- File lưu dạng `YYYY-MM-DD-slug.md` (slug sinh từ title: lowercase, a-z0-9 và `-`). Trùng tên thì thêm hậu tố `-2`, `-3`...
- Frontmatter tối thiểu: `title`, `tags`, `type`, `created` (date hôm nay). **Phải parse/ghi lại được
  frontmatter mà không phá các field lạ** (để Phase 2 thêm `review`, `ease` không vỡ).

### 3. `hi-brains search`

- `hi-brains search <query>`: ripgrep qua title/tags/nội dung; in kết quả dạng bảng
  `ngày | title | tags | đường dẫn`, sort theo ngày mới nhất trước.
- Không có query: mở fzf interactive duyệt toàn bộ note (preview nội dung), Enter → mở note bằng `$EDITOR`.
- Thiếu `ripgrep`/`fzf` binary: báo lỗi rõ ràng, gợi ý lệnh cài (brew install ...).

### 4. `hi-brains list`

- `hi-brains list` — liệt kê toàn bộ note, mới nhất trước, dạng bảng như search.
- Flags: `--tag <tag>` (lặp lại được, AND), `--type <type>`, `--recent <n>` (giới hạn n note gần nhất).

## Cấu trúc code

```
hi-brains/
├── cmd/hi-brains/main.go   # entrypoint, dispatch subcommand
├── internal/
│   ├── note/               # type Note, parse & serialize markdown + frontmatter YAML
│   ├── store/              # mở thư mục notes (BRAINS_DIR), CRUD file, sinh slug/tên file
│   └── search/             # wrapper ripgrep + fzf (exec.Command), parse output
├── go.mod
├── Makefile                # build (./bin), install ($GOBIN)
└── README.md               # cập nhật mục "Build & install" + usage Phase 1
```

- CLI parsing: dùng stdlib `flag` với subcommand dispatch, hoặc `spf13/cobra` — chọn 1, nhất quán.
- YAML: `gopkg.in/yaml.v3`. Giữ field lạ trong frontmatter khi ghi lại (map thay vì struct cứng, hoặc `yaml:",inline"` map).
- Không phụ thuộc network.

## Tiêu chí hoàn thành

- `hi-brains init && hi-brains add --quick "..." --tags go && hi-brains search <keyword>` chạy được end-to-end trên máy thật.
- Note ghi ngày hôm nay, 3 ngày sau `search` tìm lại được trong <5 giây.
- Unit test phủ: parse/serialize frontmatter (kể cả field lạ), sinh slug, filter `--tag/--type/--recent`.
- `go vet` sạch, `go test ./...` pass.

verify: go build ./... && go vet ./... && go test ./...

## Tasks gợi ý (1-5 task, đúng ràng buộc của flow)

- [ ] Khởi tạo module Go + skeleton CLI (subcommand dispatch) + lệnh `init` (tạo thư mục notes, git init)
- [ ] Package `note` + `store`: parse/serialize markdown + frontmatter YAML (giữ field lạ), sinh tên file `YYYY-MM-DD-slug.md`
- [ ] Lệnh `add`: template theo type, prompt chọn type, `--quick`, `--tags`, mở `$EDITOR`, huỷ khi nội dung rỗng
- [ ] Lệnh `search` (ripgrep, fzf interactive khi không có query) và `list` (`--tag`, `--type`, `--recent`) + bảng kết quả
- [ ] Unit test cho note/store/search-filter + Makefile build/install + cập nhật README usage

## Ngoài phạm vi (để phase sau)

Spaced repetition (SM-2), flashcard, `daily` digest, kata tracker, RSS digest, import note cũ, đồng bộ nhiều máy.
