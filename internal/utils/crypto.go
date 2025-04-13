package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

var (
	ErrInvalidKey  = errors.New("invalid encryption key")
	ErrInvalidData = errors.New("invalid data")
	ErrEncryption  = errors.New("encryption failed")
	ErrDecryption  = errors.New("decryption failed")
)

type TokenCrypto struct {
	key []byte
}

func NewTokenCrypto(key string) (*TokenCrypto, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	return &TokenCrypto{key: []byte(key)}, nil
}

func (tc *TokenCrypto) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", ErrInvalidData
	}

	block, err := aes.NewCipher(tc.key)
	if err != nil {
		return "", errors.Join(ErrEncryption, err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.Join(ErrEncryption, err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", errors.Join(ErrEncryption, err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (tc *TokenCrypto) Decrypt(encrypted string) (string, error) {
	if encrypted == "" {
		return "", ErrInvalidData
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", errors.Join(ErrDecryption, err)
	}

	block, err := aes.NewCipher(tc.key)
	if err != nil {
		return "", errors.Join(ErrDecryption, err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.Join(ErrDecryption, err)
	}

	if len(ciphertext) < gcm.NonceSize() {
		return "", ErrDecryption
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.Join(ErrDecryption, err)
	}

	return string(plaintext), nil
}
