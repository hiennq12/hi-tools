package keystore_test

import (
	"errors"
	"strings"
	"testing"

	"hi-tools/internal/llm/keystore"
)

// stubKeyStore là KeyStore giả để kiểm Resolver chuyển đúng backend enc.
type stubKeyStore struct {
	secrets map[string]string
}

func (s *stubKeyStore) Get(name string) (string, error) {
	v, ok := s.secrets[name]
	if !ok {
		return "", errors.New("không có key giả")
	}
	return v, nil
}

func (s *stubKeyStore) Set(name, secret string) error {
	s.secrets[name] = secret
	return nil
}

func TestParsePointer(t *testing.T) {
	valid := []struct {
		in   string
		kind string
		ref  string
	}{
		{"enc:deeprelay", "enc", "deeprelay"},
		{"env:MY_API_KEY", "env", "MY_API_KEY"},
		{"plain:foo", "plain", "foo"},
		{"keychain:foo", "keychain", "foo"},
	}
	for _, tc := range valid {
		kind, ref, err := keystore.ParsePointer(tc.in)
		if err != nil {
			t.Fatalf("ParsePointer(%q) lỗi: %v", tc.in, err)
		}
		if kind != tc.kind || ref != tc.ref {
			t.Fatalf("ParsePointer(%q) = (%q, %q), muốn (%q, %q)", tc.in, kind, ref, tc.kind, tc.ref)
		}
	}

	invalid := []string{"", "enc", "enc:", "bogus:x", "ENC:x", ":x", "unknown:value"}
	for _, in := range invalid {
		if _, _, err := keystore.ParsePointer(in); err == nil {
			t.Fatalf("ParsePointer(%q) = nil error, muốn lỗi", in)
		}
	}
}

func TestResolverEnv(t *testing.T) {
	t.Setenv("HI_LLM_RESOLVE_KEY", "env-secret")

	r := keystore.Resolver{}
	got, err := r.Resolve("env:HI_LLM_RESOLVE_KEY")
	if err != nil {
		t.Fatalf("Resolve env có giá trị: %v", err)
	}
	if got != "env-secret" {
		t.Fatalf("Resolve = %q, muốn env-secret", got)
	}

	t.Setenv("HI_LLM_RESOLVE_EMPTY", "")
	_, err = r.Resolve("env:HI_LLM_RESOLVE_EMPTY")
	if err == nil {
		t.Fatal("Resolve env rỗng trả nil, muốn lỗi")
	}
	if !strings.Contains(err.Error(), "HI_LLM_RESOLVE_EMPTY") || !strings.Contains(err.Error(), "chưa đặt") {
		t.Fatalf("lỗi = %q, muốn nêu biến môi trường HI_LLM_RESOLVE_EMPTY chưa đặt", err)
	}
}

func TestResolverEncUsesKeyStore(t *testing.T) {
	fake := &stubKeyStore{secrets: map[string]string{"alpha": "enc-secret"}}
	r := keystore.Resolver{Enc: fake}

	got, err := r.Resolve("enc:alpha")
	if err != nil {
		t.Fatalf("Resolve enc: %v", err)
	}
	if got != "enc-secret" {
		t.Fatalf("Resolve enc = %q, muốn enc-secret", got)
	}
}

func TestResolverUnsupportedBackends(t *testing.T) {
	r := keystore.Resolver{}
	for _, pointer := range []string{"plain:foo", "keychain:foo"} {
		t.Run(pointer, func(t *testing.T) {
			_, err := r.Resolve(pointer)
			if !errors.Is(err, keystore.ErrUnsupported) {
				t.Fatalf("Resolve(%q) lỗi = %v, muốn ErrUnsupported", pointer, err)
			}
		})
	}
}

func TestResolverUnknownPrefix(t *testing.T) {
	r := keystore.Resolver{}
	if _, err := r.Resolve("bogus:x"); err == nil {
		t.Fatal("Resolve tiền tố lạ trả nil, muốn lỗi")
	}
	if _, err := r.Resolve(""); err == nil {
		t.Fatal("Resolve con trỏ rỗng trả nil, muốn lỗi")
	}
}

func TestEnvStoreGet(t *testing.T) {
	t.Setenv("HI_LLM_ENVSTORE_KEY", "value-from-env")
	var es keystore.EnvStore

	got, err := es.Get("HI_LLM_ENVSTORE_KEY")
	if err != nil {
		t.Fatalf("EnvStore.Get: %v", err)
	}
	if got != "value-from-env" {
		t.Fatalf("EnvStore.Get = %q, muốn value-from-env", got)
	}

	t.Setenv("HI_LLM_ENVSTORE_EMPTY", "")
	_, err = es.Get("HI_LLM_ENVSTORE_EMPTY")
	if err == nil {
		t.Fatal("EnvStore.Get biến rỗng trả nil, muốn lỗi")
	}
	if !strings.Contains(err.Error(), "HI_LLM_ENVSTORE_EMPTY") {
		t.Fatalf("lỗi = %q, muốn nêu tên biến HI_LLM_ENVSTORE_EMPTY", err)
	}
}

func TestEnvStoreSetIsReadOnly(t *testing.T) {
	var es keystore.EnvStore
	if err := es.Set("HI_LLM_ENVSTORE_KEY", "x"); err == nil {
		t.Fatal("EnvStore.Set trả nil, muốn lỗi read-only")
	}
}

func TestStubStore(t *testing.T) {
	for _, kind := range []string{"plain", "keychain"} {
		stub := keystore.StubStore{Kind: kind}
		if _, err := stub.Get("foo"); !errors.Is(err, keystore.ErrUnsupported) {
			t.Fatalf("StubStore{%q}.Get lỗi = %v, muốn ErrUnsupported", kind, err)
		}
		if err := stub.Set("foo", "bar"); !errors.Is(err, keystore.ErrUnsupported) {
			t.Fatalf("StubStore{%q}.Set lỗi = %v, muốn ErrUnsupported", kind, err)
		}
	}
}
