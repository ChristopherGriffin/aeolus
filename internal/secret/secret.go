// Package secret seals secret field values, such as Wi-Fi passphrases, before
// they enter the change log (0027). Sealing uses AES-256-GCM with one key that
// lives only on the manager host.
//
// A sealed value is the JSON object {"$sealed": "<base64 nonce+ciphertext>"}.
// Each value is bound to its field path, so a sealed passphrase cannot be
// copied into a readable field and opened there.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

const sealedKey = "$sealed"

var (
	ErrNotSealed = errors.New("value is not sealed")
	ErrBadKey    = errors.New("secret key must be 32 bytes, base64-encoded")
	ErrKeyPerms  = errors.New("secret key file must not be readable by group or others")
)

// Box seals and opens values with one key.
type Box struct{ aead cipher.AEAD }

// New returns a Box for a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, ErrBadKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// LoadOrCreate reads the key file at path, creating it with a fresh random
// key if it does not exist. Only setup should create a key: a new key cannot
// open anything sealed with the old one.
func LoadOrCreate(path string) (*Box, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		enc := base64.StdEncoding.EncodeToString(key) + "\n"
		if err := os.WriteFile(path, []byte(enc), 0o600); err != nil {
			return nil, err
		}
		return New(key)
	}
	return Load(path)
}

// Load reads an existing key file. It refuses a file readable by group or
// others.
func Load(path string) (*Box, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("%w: %s is %v", ErrKeyPerms, path, info.Mode().Perm())
		}
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, ErrBadKey
	}
	return New(key)
}

// Seal encrypts v, bound to the field path it will be stored under.
func (b *Box) Seal(path string, v any) (map[string]any, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := b.aead.Seal(nonce, nonce, plain, []byte(path))
	return map[string]any{sealedKey: base64.StdEncoding.EncodeToString(out)}, nil
}

// Open decrypts a value sealed for path.
func (b *Box) Open(path string, v any) (any, error) {
	s, ok := sealedText(v)
	if !ok {
		return nil, ErrNotSealed
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return nil, fmt.Errorf("sealed value for %s is malformed", path)
	}
	n := b.aead.NonceSize()
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], []byte(path))
	if err != nil {
		return nil, fmt.Errorf("sealed value for %s does not open with this key", path)
	}
	var out any
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// IsSealed reports whether v is a sealed value.
func IsSealed(v any) bool {
	_, ok := sealedText(v)
	return ok
}

func sealedText(v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return "", false
	}
	s, ok := m[sealedKey].(string)
	return s, ok
}
