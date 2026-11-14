package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// promptLine hỏi một giá trị dạng text và đọc từ in.
func promptLine(in *bufio.Reader, stderr io.Writer, label string) (string, error) {
	fmt.Fprint(stderr, label)
	line, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// confirm hỏi câu yes/no; mặc định là no khi người dùng nhập rỗng.
func confirm(in *bufio.Reader, stderr io.Writer, q string) (bool, error) {
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
