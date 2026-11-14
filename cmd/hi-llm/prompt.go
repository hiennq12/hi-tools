package main

import (
	"fmt"
	"io"
	"strings"
)

// readLine đọc từng byte tới '\n', không đọc trước như bufio: phần còn lại của stdin
// (vd dán nhiều dòng) vẫn nằm nguyên cho term.ReadPassword đọc tiếp trên cùng fd.
func readLine(in io.Reader) (string, error) {
	var line []byte
	b := make([]byte, 1)
	for {
		n, err := in.Read(b)
		if n == 1 {
			if b[0] == '\n' {
				return string(line), nil
			}
			line = append(line, b[0])
		}
		if err != nil {
			if err == io.EOF {
				return string(line), nil
			}
			return "", err
		}
	}
}

// promptLine hỏi một giá trị dạng text và đọc từ in.
func promptLine(in io.Reader, stderr io.Writer, label string) (string, error) {
	fmt.Fprint(stderr, label)
	line, err := readLine(in)
	return strings.TrimSpace(line), err
}

// confirm hỏi câu yes/no; mặc định là no khi người dùng nhập rỗng.
func confirm(in io.Reader, stderr io.Writer, q string) (bool, error) {
	line, err := promptLine(in, stderr, q)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(line) {
	case "y", "yes", "c", "co", "có":
		return true, nil
	}
	return false, nil
}
