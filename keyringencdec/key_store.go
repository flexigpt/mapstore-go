package keyringencdec

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

var ErrNotFound = errors.New("keystore: secret not found")

type KeyStore interface {
	Get(service, username string) (string, error)
	Set(service, username, secret string) error
}

type osKeyStore struct{}

func (osKeyStore) Get(service, username string) (string, error) {
	secret, err := keyring.Get(service, username)
	if err == nil {
		return secret, nil
	}

	// Translate implementation-specific errors to package errors.
	if errors.Is(err, keyring.ErrNotFound) {
		// Wrap so errors.Is(err, ErrNotFound) works.
		return "", fmt.Errorf("%w", ErrNotFound)
	}

	// Everything else: return wrapped.
	return "", fmt.Errorf("keystore get %q/%q: %w", service, username, err)
}

func (osKeyStore) Set(service, username, secret string) error {
	if err := keyring.Set(service, username, secret); err != nil {
		return fmt.Errorf("keystore set %q/%q: %w", service, username, err)
	}
	return nil
}
