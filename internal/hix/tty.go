package hix

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// ErrNoTTY trả về khi cần đọc dữ liệu tương tác nhưng stdin không phải terminal.
var ErrNoTTY = errors.New("không có terminal")

// IsTTY trả về true nếu r là *os.File gắn với terminal.
func IsTTY(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// ReadPassword in prompt ra stderr rồi đọc một dòng từ terminal với ký tự bị ẩn.
// Không có TTY thì trả ErrNoTTY, không bao giờ treo.
func ReadPassword(prompt string, stdin io.Reader, stderr io.Writer) (string, error) {
	if !IsTTY(stdin) {
		return "", ErrNoTTY
	}
	f := stdin.(*os.File)
	if prompt != "" {
		fmt.Fprint(stderr, prompt)
	}
	b, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
