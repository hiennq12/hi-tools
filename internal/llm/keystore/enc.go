package keystore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"

	"hi-tools/internal/hix"
)

const (
	magic        = "HLLM"
	version byte = 0x01
	saltLen      = 16
	nonceLen     = 12
	keyLen       = 32

	// EnvMasterPassword là master password không tương tác.
	EnvMasterPassword = "HI_LLM_MASTER_PASSWORD"
)

// ErrBadPassword là lỗi chung cho sai password / file hỏng / magic hoặc version lạ.
var ErrBadPassword = errors.New("master password sai hoặc keys.enc hỏng")

// ErrNoKeysFile trả về khi keys.enc chưa tồn tại.
var ErrNoKeysFile = errors.New("chưa có keys.enc, chạy hi-llm add")

// ErrNoPassword trả về khi không có env và cũng không có terminal để hỏi.
var ErrNoPassword = errors.New("cần HI_LLM_MASTER_PASSWORD hoặc terminal để nhập master password")

// PasswordFunc trả về master password; confirm=true khi keys.enc được tạo mới.
type PasswordFunc func(confirm bool) (string, error)

// KeysPath trả về đường dẫn keys.enc trong home.
func KeysPath(home string) string {
	return filepath.Join(home, "keys.enc")
}

// Encrypt mã hoá plain bằng AES-256-GCM với khoá argon2id và salt/nonce mới.
func Encrypt(plain []byte, password string) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("sinh salt: %w", err)
	}
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("sinh nonce: %w", err)
	}
	return encryptWith(plain, password, salt, nonce)
}

func encryptWith(plain []byte, password string, salt, nonce []byte) ([]byte, error) {
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, keyLen)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(magic)+1+saltLen+nonceLen+len(plain)+gcm.Overhead())
	out = append(out, magic...)
	out = append(out, version)
	out = append(out, salt...)
	out = append(out, nonce...)
	out = gcm.Seal(out, nonce, plain, nil)
	return out, nil
}

// Decrypt giải mã file HLLM v1. Mọi lỗi đều quy về ErrBadPassword.
func Decrypt(data []byte, password string) ([]byte, error) {
	headerLen := len(magic) + 1 + saltLen + nonceLen
	if len(data) < headerLen {
		return nil, ErrBadPassword
	}
	if string(data[:len(magic)]) != magic {
		return nil, ErrBadPassword
	}
	if data[len(magic)] != version {
		return nil, ErrBadPassword
	}
	off := len(magic) + 1
	salt := data[off : off+saltLen]
	off += saltLen
	nonce := data[off : off+nonceLen]
	off += nonceLen
	ciphertext := data[off:]

	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, keyLen)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrBadPassword
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrBadPassword
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrBadPassword
	}
	return plain, nil
}

// EncStore là backend enc: keys.enc + master password.
type EncStore struct {
	Path     string
	Password PasswordFunc
}

// NewEncStore tạo EncStore; pw nil thì dùng EnvPassword().
func NewEncStore(path string, pw PasswordFunc) *EncStore {
	if pw == nil {
		pw = EnvPassword()
	}
	return &EncStore{Path: path, Password: pw}
}

func (s *EncStore) password(confirm bool) (string, error) {
	if s.Password == nil {
		return "", ErrNoPassword
	}
	return s.Password(confirm)
}

// load đọc và giải mã toàn bộ entries. File chưa có trả ErrNoKeysFile.
func (s *EncStore) load() (map[string]string, []byte, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, ErrNoKeysFile
		}
		return nil, nil, err
	}
	pw, err := s.password(false)
	if err != nil {
		return nil, nil, err
	}
	plain, err := Decrypt(data, pw)
	if err != nil {
		return nil, nil, err
	}
	var entries map[string]string
	if err := json.Unmarshal(plain, &entries); err != nil {
		return nil, nil, ErrBadPassword
	}
	if entries == nil {
		entries = map[string]string{}
	}
	return entries, data, nil
}

// Get trả secret theo tên. File chưa có → ErrNoKeysFile (không hỏi password).
func (s *EncStore) Get(name string) (string, error) {
	if _, err := os.Stat(s.Path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ErrNoKeysFile
		}
		return "", err
	}
	entries, _, err := s.load()
	if err != nil {
		return "", err
	}
	v, ok := entries[name]
	if !ok {
		return "", fmt.Errorf("không có key '%s' trong keys.enc", name)
	}
	return v, nil
}

// Set thêm/cập nhật một key rồi mã hoá lại toàn bộ file.
// Nếu file đang hỏng/sai password thì không ghi đè.
func (s *EncStore) Set(name, secret string) error {
	var (
		entries map[string]string
		pw      string
	)

	data, err := os.ReadFile(s.Path)
	switch {
	case err == nil:
		var perr error
		pw, perr = s.password(false)
		if perr != nil {
			return perr
		}
		plain, derr := Decrypt(data, pw)
		if derr != nil {
			return derr
		}
		if uerr := json.Unmarshal(plain, &entries); uerr != nil {
			return ErrBadPassword
		}
	case errors.Is(err, fs.ErrNotExist):
		pw, err = s.password(true)
		if err != nil {
			return err
		}
	default:
		return err
	}
	if entries == nil {
		entries = map[string]string{}
	}
	entries[name] = secret

	plain, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	out, err := Encrypt(plain, pw)
	if err != nil {
		return err
	}
	return hix.WriteFileAtomic(s.Path, out, 0o600)
}

// EnvPassword trả PasswordFunc chỉ đọc env HI_LLM_MASTER_PASSWORD.
func EnvPassword() PasswordFunc {
	return func(bool) (string, error) {
		if pw := os.Getenv(EnvMasterPassword); pw != "" {
			return pw, nil
		}
		return "", ErrNoPassword
	}
}

// PromptPassword ưu tiên env, sau đó hỏi terminal (ẩn ký tự).
// confirm=true thì hỏi hai lần và bắt buộc khớp.
func PromptPassword(stdin io.Reader, stderr io.Writer) PasswordFunc {
	return func(confirm bool) (string, error) {
		if pw := os.Getenv(EnvMasterPassword); pw != "" {
			return pw, nil
		}
		if !hix.IsTTY(stdin) {
			return "", ErrNoPassword
		}
		pw, err := hix.ReadPassword("Master password: ", stdin, stderr)
		if err != nil {
			return "", err
		}
		if confirm {
			again, err := hix.ReadPassword("Nhập lại master password: ", stdin, stderr)
			if err != nil {
				return "", err
			}
			if subtle.ConstantTimeCompare([]byte(pw), []byte(again)) != 1 {
				return "", errors.New("master password nhập lại không khớp")
			}
		}
		return pw, nil
	}
}
