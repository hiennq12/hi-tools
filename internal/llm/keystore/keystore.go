// Package keystore quản lý API key: interface chung + backend enc/env/plain/keychain.
package keystore

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// KeyStore là backend lưu secret theo tên.
type KeyStore interface {
	Get(name string) (string, error)
	Set(name, secret string) error
}

// ErrUnsupported trả về cho các backend chưa làm ở phase 1.
var ErrUnsupported = errors.New("chưa hỗ trợ ở phase 1")

// ParsePointer tách con trỏ "kind:ref" (enc|env|plain|keychain).
func ParsePointer(p string) (kind, ref string, err error) {
	idx := strings.IndexByte(p, ':')
	if idx <= 0 {
		return "", "", fmt.Errorf("con trỏ key %q không hợp lệ (cần enc:|env:|plain:|keychain:)", p)
	}
	kind, ref = p[:idx], p[idx+1:]
	switch kind {
	case "enc", "env", "plain", "keychain":
	default:
		return "", "", fmt.Errorf("con trỏ key %q có tiền tố lạ %q", p, kind)
	}
	if ref == "" {
		return "", "", fmt.Errorf("con trỏ key %q thiếu tên sau tiền tố", p)
	}
	return kind, ref, nil
}

// Resolver giải con trỏ key ra secret thật.
type Resolver struct {
	Enc KeyStore
}

// Resolve chọn backend theo tiền tố của pointer.
func (r Resolver) Resolve(pointer string) (string, error) {
	kind, ref, err := ParsePointer(pointer)
	if err != nil {
		return "", err
	}
	switch kind {
	case "enc":
		if r.Enc == nil {
			return "", errors.New("keystore enc chưa được cấu hình")
		}
		return r.Enc.Get(ref)
	case "env":
		return EnvStore{}.Get(ref)
	case "plain", "keychain":
		return StubStore{Kind: kind}.Get(ref)
	default:
		return "", fmt.Errorf("con trỏ key %q có tiền tố lạ %q", pointer, kind)
	}
}

// EnvStore đọc secret từ biến môi trường (read-only).
type EnvStore struct{}

// Get đọc os.Getenv; biến rỗng coi như chưa đặt.
func (EnvStore) Get(name string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", fmt.Errorf("biến môi trường %s chưa đặt", name)
	}
	return v, nil
}

// Set luôn lỗi vì env store là read-only.
func (EnvStore) Set(name, secret string) error {
	return fmt.Errorf("backend env:%s là read-only, không ghi được", name)
}

// StubStore là backend chưa hỗ trợ ở phase 1.
type StubStore struct {
	Kind string
}

// Get luôn trả ErrUnsupported.
func (s StubStore) Get(name string) (string, error) {
	return "", fmt.Errorf("%s:%s: %w", s.Kind, name, ErrUnsupported)
}

// Set luôn trả ErrUnsupported.
func (s StubStore) Set(name, secret string) error {
	return fmt.Errorf("%s:%s: %w", s.Kind, name, ErrUnsupported)
}
