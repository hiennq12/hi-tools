package keystore_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hi-tools/internal/llm/keystore"
)

func fixedPassword(password string) keystore.PasswordFunc {
	return func(bool) (string, error) { return password, nil }
}

// tamper trả bản sao của data với byte ở idx bị đổi thành b.
func tamper(data []byte, idx int, b byte) []byte {
	out := append([]byte(nil), data...)
	out[idx] = b
	return out
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	plain := []byte(`{"alpha":"beta"}`)

	data, err := keystore.Encrypt(plain, "master")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(data) < 4+1+16+12 {
		t.Fatalf("dữ liệu mã hoá chỉ có %d byte, muốn ít nhất header HLLM v1 + salt + nonce", len(data))
	}
	if got := string(data[:4]); got != "HLLM" {
		t.Fatalf("magic = %q, muốn HLLM", got)
	}
	if data[4] != 0x01 {
		t.Fatalf("version = %d, muốn 1", data[4])
	}
	if bytes.Contains(data, plain) {
		t.Fatal("ciphertext chứa plaintext")
	}

	got, err := keystore.Decrypt(data, "master")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("plaintext = %q, muốn %q", got, plain)
	}
}

func TestEncryptUsesFreshSaltAndNonce(t *testing.T) {
	plain := []byte(`{"alpha":"beta"}`)

	first, err := keystore.Encrypt(plain, "master")
	if err != nil {
		t.Fatalf("Encrypt lần 1: %v", err)
	}
	second, err := keystore.Encrypt(plain, "master")
	if err != nil {
		t.Fatalf("Encrypt lần 2: %v", err)
	}

	if bytes.Equal(first, second) {
		t.Fatal("hai lần Encrypt cùng dữ liệu ra cùng ciphertext (thiếu salt/nonce mới)")
	}
	if bytes.Equal(first[5:21], second[5:21]) {
		t.Fatal("salt không đổi giữa hai lần ghi")
	}
	if bytes.Equal(first[21:33], second[21:33]) {
		t.Fatal("nonce không đổi giữa hai lần ghi")
	}
}

func TestDecryptBadDataReturnsErrBadPassword(t *testing.T) {
	valid, err := keystore.Encrypt([]byte(`{"a":"b"}`), "master")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	cases := []struct {
		name string
		data []byte
	}{
		{"magic sai", tamper(valid, 0, 'X')},
		{"version lạ", tamper(valid, 4, 0x02)},
		{"file hỏng quá ngắn", append([]byte(nil), valid[:10]...)},
		{"ciphertext hỏng", tamper(valid, len(valid)-1, valid[len(valid)-1]^0xff)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := keystore.Decrypt(tc.data, "master"); !errors.Is(err, keystore.ErrBadPassword) {
				t.Fatalf("Decrypt lỗi = %v, muốn errors.Is(err, ErrBadPassword)", err)
			}
		})
	}

	if _, err := keystore.Decrypt(valid, "wrong-password"); !errors.Is(err, keystore.ErrBadPassword) {
		t.Fatalf("Decrypt sai password lỗi = %v, muốn ErrBadPassword", err)
	}
}

func TestEncStoreLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.enc")
	var confirms []bool
	store := keystore.NewEncStore(path, func(confirm bool) (string, error) {
		confirms = append(confirms, confirm)
		return "master", nil
	})

	if err := store.Set("alpha", "secret-alpha"); err != nil {
		t.Fatalf("Set lần đầu (tạo mới): %v", err)
	}
	if len(confirms) == 0 || !confirms[0] {
		t.Fatalf("lần tạo keys.enc mới phải gọi Password(true), nhận %v", confirms)
	}

	got, err := store.Get("alpha")
	if err != nil {
		t.Fatalf("Get sau Set: %v", err)
	}
	if got != "secret-alpha" {
		t.Fatalf("Get = %q, muốn secret-alpha", got)
	}

	// Thêm key mới phải giữ key cũ.
	if err := store.Set("beta", "secret-beta"); err != nil {
		t.Fatalf("Set key thứ hai: %v", err)
	}
	for name, want := range map[string]string{"alpha": "secret-alpha", "beta": "secret-beta"} {
		got, err := store.Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		if got != want {
			t.Fatalf("Get(%q) = %q, muốn %q", name, got, want)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat keys.enc: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("quyền keys.enc = %o, muốn 600", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc keys.enc: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("HLLM")) {
		t.Fatalf("keys.enc không bắt đầu bằng magic HLLM: %q", data[:4])
	}

	plain, err := keystore.Decrypt(data, "master")
	if err != nil {
		t.Fatalf("Decrypt nội dung keys.enc: %v", err)
	}
	var entries map[string]string
	if err := json.Unmarshal(plain, &entries); err != nil {
		t.Fatalf("plaintext của keys.enc không phải JSON: %v (%q)", err, plain)
	}
	if entries["alpha"] != "secret-alpha" || entries["beta"] != "secret-beta" {
		t.Fatalf("plaintext = %v, muốn chứa alpha và beta", entries)
	}
}

func TestEncStoreFileHasNoPlaintextSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.enc")
	store := keystore.NewEncStore(path, fixedPassword("master"))

	const secret = "sk-PLAINTEXT-MUST-NOT-APPEAR"
	if err := store.Set("provider", secret); err != nil {
		t.Fatalf("Set: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc keys.enc: %v", err)
	}
	if bytes.Contains(data, []byte(secret)) {
		t.Fatal("keys.enc chứa plaintext secret")
	}
	if bytes.Contains(data, []byte(`"provider"`)) {
		t.Fatal("keys.enc chứa tên key dạng plaintext")
	}
}

func TestEncStoreRewritesWithFreshSaltAndNonce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.enc")
	store := keystore.NewEncStore(path, fixedPassword("master"))

	if err := store.Set("alpha", "a"); err != nil {
		t.Fatalf("Set lần 1: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc keys.enc lần 1: %v", err)
	}
	if err := store.Set("beta", "b"); err != nil {
		t.Fatalf("Set lần 2: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc keys.enc lần 2: %v", err)
	}

	if len(first) < 33 || len(second) < 33 {
		t.Fatalf("file quá ngắn (lần 1 %d byte, lần 2 %d byte)", len(first), len(second))
	}
	if bytes.Equal(first, second) {
		t.Fatal("ghi lại keys.enc không thay đổi nội dung (thiếu salt/nonce mới)")
	}
	if bytes.Equal(first[5:21], second[5:21]) {
		t.Fatal("salt không đổi giữa hai lần ghi")
	}
	if bytes.Equal(first[21:33], second[21:33]) {
		t.Fatal("nonce không đổi giữa hai lần ghi")
	}
}

func TestEncStoreGetWithoutKeysFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.enc")
	prompted := false
	store := keystore.NewEncStore(path, func(bool) (string, error) {
		prompted = true
		return "master", nil
	})

	_, err := store.Get("alpha")
	if !errors.Is(err, keystore.ErrNoKeysFile) {
		t.Fatalf("Get khi chưa có keys.enc lỗi = %v, muốn ErrNoKeysFile", err)
	}
	if prompted {
		t.Fatal("Get đã hỏi password dù keys.enc chưa tồn tại")
	}
}

func TestEncStoreGetMissingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.enc")
	store := keystore.NewEncStore(path, fixedPassword("master"))
	if err := store.Set("alpha", "a"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	_, err := store.Get("beta")
	if err == nil {
		t.Fatal("Get key không tồn tại trả nil, muốn lỗi")
	}
	if !strings.Contains(err.Error(), "không có key") || !strings.Contains(err.Error(), "beta") {
		t.Fatalf("lỗi Get = %q, muốn nêu không có key 'beta'", err)
	}
}

func TestEncStoreWrongPasswordDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.enc")
	good := keystore.NewEncStore(path, fixedPassword("master"))
	if err := good.Set("alpha", "a"); err != nil {
		t.Fatalf("Set bằng password đúng: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc keys.enc: %v", err)
	}

	bad := keystore.NewEncStore(path, fixedPassword("wrong"))
	if _, err := bad.Get("alpha"); !errors.Is(err, keystore.ErrBadPassword) {
		t.Fatalf("Get với password sai lỗi = %v, muốn ErrBadPassword", err)
	}
	if err := bad.Set("alpha", "b"); !errors.Is(err, keystore.ErrBadPassword) {
		t.Fatalf("Set với password sai lỗi = %v, muốn ErrBadPassword", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc lại keys.enc: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("keys.enc bị ghi đè dù password sai")
	}
}

func TestEncStoreCorruptFileNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.enc")
	corrupt := []byte("khong-phai-file-ma-hoa")
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatalf("ghi file hỏng: %v", err)
	}

	store := keystore.NewEncStore(path, fixedPassword("master"))
	if _, err := store.Get("alpha"); !errors.Is(err, keystore.ErrBadPassword) {
		t.Fatalf("Get trên file hỏng lỗi = %v, muốn ErrBadPassword", err)
	}
	if err := store.Set("alpha", "a"); !errors.Is(err, keystore.ErrBadPassword) {
		t.Fatalf("Set trên file hỏng lỗi = %v, muốn ErrBadPassword", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("đọc lại file hỏng: %v", err)
	}
	if !bytes.Equal(corrupt, after) {
		t.Fatal("file hỏng bị ghi đè")
	}
}

func TestEncStoreSetCreatesMissingDirs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chua", "co", "config")
	path := filepath.Join(dir, "keys.enc")
	store := keystore.NewEncStore(path, fixedPassword("master"))

	if err := store.Set("alpha", "a"); err != nil {
		t.Fatalf("Set vào thư mục chưa tồn tại: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("keys.enc không được tạo: %v", err)
	}

	got, err := store.Get("alpha")
	if err != nil {
		t.Fatalf("Get sau Set: %v", err)
	}
	if got != "a" {
		t.Fatalf("Get = %q, muốn a", got)
	}
}

func TestEnvPassword(t *testing.T) {
	// Đảm bảo biến thực sự chưa được đặt cho nhánh lỗi.
	if err := os.Unsetenv(keystore.EnvMasterPassword); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	t.Cleanup(func() { _ = os.Unsetenv(keystore.EnvMasterPassword) })

	if _, err := keystore.EnvPassword()(false); !errors.Is(err, keystore.ErrNoPassword) {
		t.Fatalf("EnvPassword khi thiếu env lỗi = %v, muốn ErrNoPassword", err)
	}

	t.Setenv(keystore.EnvMasterPassword, "master-from-env")
	got, err := keystore.EnvPassword()(true)
	if err != nil {
		t.Fatalf("EnvPassword khi có env: %v", err)
	}
	if got != "master-from-env" {
		t.Fatalf("password = %q, muốn master-from-env", got)
	}
}
