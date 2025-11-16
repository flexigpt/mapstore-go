// Package keyringencdec provides a value encoder/decoder backed by the OS keyring.
// It encrypts string values with AES-256-GCM and persists the key in the OS keyring.
// Ensure a compatible keyring backend is available in headless/container environments.
package keyringencdec

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/zalando/go-keyring"
)

// EncryptedStringValueEncoderDecoder uses AES-256-GCM + base64 for encoding/decoding
// and persists the AES key in the OS keyring under the configured service/username.
type EncryptedStringValueEncoderDecoder struct {
	service  string
	username string
	debug    bool
}

// Option is a functional option for configuring EncryptedStringValueEncoderDecoder.
type Option func(*EncryptedStringValueEncoderDecoder)

// WithDebug sets debug mode.
func WithDebug(debug bool) Option {
	return func(e *EncryptedStringValueEncoderDecoder) {
		e.debug = debug
	}
}

// NewEncryptedStringValueEncoderDecoder constructs a new instance.
func NewEncryptedStringValueEncoderDecoder(
	service, username string,
	opts ...Option,
) (*EncryptedStringValueEncoderDecoder, error) {
	if service == "" || username == "" {
		return nil, errors.New("empty service or username")
	}
	e := &EncryptedStringValueEncoderDecoder{
		service:  service,
		username: username,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	return e, nil
}

func (e *EncryptedStringValueEncoderDecoder) Encode(w io.Writer, value any) error {
	v, ok := value.(string)
	if !ok {
		return errors.New("got non string encode input")
	}
	encryptedData, err := e.encryptString(v)
	if err != nil {
		return err
	}

	_, err = w.Write([]byte(encryptedData))
	return err
}

func (e *EncryptedStringValueEncoderDecoder) Decode(r io.Reader, value any) error {
	encryptedData, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	decryptedData, err := e.decryptString(string(encryptedData))
	if err != nil {
		return err
	}

	// Use reflection to handle the value.
	valuePtr := reflect.ValueOf(value)

	// Check if value is a pointer.
	if valuePtr.Kind() != reflect.Ptr {
		return fmt.Errorf("value must be a pointer. Kind: %v", valuePtr.Kind())
	}

	// Dereference the pointer to get the underlying value.
	valueElem := valuePtr.Elem()

	// If the underlying value is an interface, set the decrypted data directly.
	if valueElem.Kind() == reflect.Interface {
		valueElem.Set(reflect.ValueOf(decryptedData))
		return nil
	}

	// Otherwise, check if the underlying value is a string.
	if valueElem.Kind() != reflect.String {
		return fmt.Errorf(
			"value must be a pointer to a string or interface. Kind: %v",
			valueElem.Kind(),
		)
	}

	// Set the decrypted data to the dereferenced value.
	valueElem.SetString(decryptedData)

	return nil
}

// encryptString encrypts a string using AES-256-GCM and returns a base64-encoded string.
func (e *EncryptedStringValueEncoderDecoder) encryptString(plaintext string) (string, error) {
	key, err := e.getKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM block cipher mode: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	return encoded, nil
}

// decryptString decrypts a base64-encoded string that was encrypted using AES-256-GCM.
func (e *EncryptedStringValueEncoderDecoder) decryptString(encodedCiphertext string) (string, error) {
	key, err := e.getKey()
	if err != nil {
		return "", err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM block cipher mode: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt ciphertext: %w", err)
	}
	return string(plaintext), nil
}

// getKey retrieves or generates an AES-256 encryption key from the keyring.
// If the key does not exist, it generates a new one, stores it, and returns it.
func (e *EncryptedStringValueEncoderDecoder) getKey() ([]byte, error) {
	// AES-256 requires a 32-byte key.
	const keySize = 32

	// Attempt to retrieve the key from the keyring.
	keyStr, err := keyring.Get(e.service, e.username)

	switch {
	case err == nil:
		// Decode the base64-encoded key.
		key, err := base64.StdEncoding.DecodeString(keyStr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode key: %w", err)
		}
		if len(key) != keySize {
			return nil, fmt.Errorf("unexpected key length: got %d, want %d", len(key), keySize)
		}
		return key, nil
	case errors.Is(err, keyring.ErrNotFound):
		// Generate a new 32-byte key if not found.
		key := make([]byte, keySize)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			return nil, fmt.Errorf("failed to generate key: %w", err)
		}
		// Store the key in the keyring.
		keyStr := base64.StdEncoding.EncodeToString(key)
		if err := keyring.Set(e.service, e.username, keyStr); err != nil {
			return nil, fmt.Errorf("failed to store key in keyring: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("failed to retrieve key from keyring: %w", err)
	}
}
