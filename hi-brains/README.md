# hi-brains

"Bộ não thứ hai" cho lập trình viên — CLI ghi nhận và ôn luyện kiến thức kỹ thuật cá nhân.

## Ý tưởng

Ghi note mà không ôn thì 3 tháng sau cũng quên; học mà không ghi lại thì mỗi lần gặp lại phải tìm từ đầu. `hi-brains` khép kín vòng lặp:

```
Ghi nhận → Tổ chức → Tìm kiếm → Ôn luyện → Digest
```

- **Ghi nhận**: lỗi đã fix, quyết định kiến trúc (ADR), concept, bài kata, snippet.
- **Tổ chức**: markdown thuần + frontmatter YAML, lưu trong 1 thư mục git (`~/brains/`) — không lock-in, diff được, backup qua git.
- **Tìm kiếm**: ripgrep + fzf, tìm lại note trong vài giây.
- **Ôn luyện** (Phase 2): spaced repetition (SM-2) ngay trong terminal.
- **Digest** (Phase 3): bản tin buổi sáng — cards đến hạn + note mới.

## Lộ trình

| Phase | Nội dung | Trạng thái |
|---|---|---|
| 1 | `init`, `add`, `search`, `list` — ghi note nhanh, tìm lại được ngay | ⬅ đang làm |
| 2 | `review` — spaced repetition (SM-2) + flashcard sinh từ note solution | chưa |
| 3 | `daily` — digest buổi sáng (cards đến hạn + note mới) | chưa |
| 4 | Kata tracker + streak, RSS digest theo tag | chưa |

Chi tiết Phase 1: xem [SPEC-phase1.md](SPEC-phase1.md).

## Cấu trúc repo (monorepo `hi-tools` — module Go chung ở gốc)

```
hi-tools/
├── go.mod                    # module hi-tools
├── Makefile                  # make build/install (mọi binary trong cmd/)
├── cmd/hi-brains/            # main.go, các subcommand
├── internal/
│   ├── brains/
│   │   ├── note/             # parse/serialize markdown + frontmatter YAML
│   │   ├── store/            # đọc/ghi thư mục notes, index tag, đặt tên file
│   │   └── search/           # gọi ripgrep + fzf qua shell
│   └── hix/                  # hạ tầng CLI dùng chung giữa các tool (fzf, $EDITOR, table...)
└── hi-brains/                # docs của tool này
    ├── README.md
    └── SPEC-phase1.md
```

## Format note

Mỗi note là 1 file markdown, tên dạng `YYYY-MM-DD-slug.md`:

```markdown
---
title: Redis timeout khi connection pool cạn
tags: [redis, go, debugging]
type: solution        # solution | adr | concept | kata | snippet
created: 2026-11-13
---

## Triệu chứng
...

## Nguyên nhân
...

## Cách fix
...
```

Phase 2 sẽ bổ sung field `review` (ngày ôn kế tiếp) và `ease` (hệ số SM-2) vào frontmatter — format Phase 1 phải tương thích sẵn.

## Ghi chú sử dụng

- Thư mục notes mặc định: `~/brains/`, override bằng env `BRAINS_DIR`.
- Editor: mở `$EDITOR` (fallback `vi`) như git commit.
- Phụ thuộc runtime: `ripgrep` (bắt buộc cho search), `fzf` (tùy chọn, cho chế độ interactive).

## Build & install

Chạy từ gốc repo `hi-tools/` (Makefile chung):

```sh
make build              # build mọi binary vào ./bin
make install            # copy vào $GOBIN
make build hi-brains    # chỉ build tool này
```
