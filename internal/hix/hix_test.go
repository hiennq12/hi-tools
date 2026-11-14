package hix_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hi-tools/internal/hix"
)

func TestIsTTYOnNonFile(t *testing.T) {
	cases := []struct {
		name string
		in   io.Reader
	}{
		{"string reader", strings.NewReader("hello")},
		{"byte buffer", bytes.NewBufferString("hello")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if hix.IsTTY(tc.in) {
				t.Fatalf("IsTTY(%T) = true, muốn false", tc.in)
			}
		})
	}
}

func TestIsTTYOnPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	if hix.IsTTY(r) {
		t.Fatal("IsTTY trên pipe = true, muốn false (pipe không phải terminal)")
	}
}

func TestReadPasswordWithoutTTY(t *testing.T) {
	var stderr bytes.Buffer
	got, err := hix.ReadPassword("master password: ", strings.NewReader("secret\n"), &stderr)
	if err == nil {
		t.Fatalf("ReadPassword không có TTY trả về %q, muốn lỗi", got)
	}
	if !errors.Is(err, hix.ErrNoTTY) {
		t.Fatalf("ReadPassword lỗi = %v, muốn errors.Is(err, ErrNoTTY)", err)
	}
}

func TestTableRender(t *testing.T) {
	tbl := hix.NewTable("NAME", "KIND")
	tbl.Add("alpha", "openai")
	tbl.Add("bb", "sub")

	var buf bytes.Buffer
	if err := tbl.Render(&buf); err != nil {
		t.Fatalf("Table.Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"NAME", "KIND", "alpha", "openai", "bb", "sub"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output bảng thiếu %q:\n%s", want, out)
		}
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("bảng có %d dòng, muốn 3:\n%s", len(lines), out)
	}

	// tabwriter phải căn cột: cột thứ hai của header và của các row bắt đầu cùng vị trí.
	wantCol := strings.Index(lines[0], "KIND")
	if wantCol < 0 {
		t.Fatalf("không thấy KIND trong header: %q", lines[0])
	}
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("dòng %q không có đúng 2 cột", line)
		}
		if got := strings.Index(line, fields[1]); got != wantCol {
			t.Fatalf("cột 2 của %q bắt đầu ở %d, header ở %d:\n%s", line, got, wantCol, out)
		}
	}
}

func TestWriteFileAtomicCreatesParentsAndPerm(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "a", "b")
	path := filepath.Join(dir, "providers.yaml")
	want := []byte("providers: {}\n")

	if err := hix.WriteFileAtomic(path, want, 0o600); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc file vừa ghi: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("nội dung = %q, muốn %q", got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("quyền file = %o, muốn 600", perm)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat thư mục cha: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("quyền thư mục cha = %o, muốn 700", perm)
	}

	// Ghi đè phải thay nội dung và giữ đúng quyền mới.
	if err := hix.WriteFileAtomic(path, []byte("new\n"), 0o640); err != nil {
		t.Fatalf("WriteFileAtomic ghi đè: %v", err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc file sau ghi đè: %v", err)
	}
	if string(got) != "new\n" {
		t.Fatalf("nội dung sau ghi đè = %q", got)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("stat file sau ghi đè: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o640 {
		t.Fatalf("quyền file sau ghi đè = %o, muốn 640", perm)
	}

	// Không được để lại file tạm.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("đọc thư mục: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("thư mục còn %d file (%v), muốn đúng 1", len(entries), names)
	}
}
