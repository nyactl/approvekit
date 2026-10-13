package keychain

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// SecretService stores tokens through the freedesktop.org Secret Service
// API (GNOME Keyring, KWallet), in the user's default collection. There is
// no fallback to files or environment variables: without a running Secret
// Service, storing and reading fail.
//
// Unlike the macOS Keychain, the Secret Service does not tie an item to the
// program that created it: any program of the user's unlocked session can
// read it.
type SecretService struct {
	service string
}

// New returns a Store in the Secret Service. service names the items, such
// as the tool's name.
func New(service string) Store { return SecretService{service: service} }

func (s SecretService) Get(account string) (string, error) {
	token, err := keyring.Get(s.service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("reading token from the Secret Service (is GNOME Keyring or KWallet running and unlocked?): %w", err)
	}
	return token, nil
}

func (s SecretService) Set(account, token string) error {
	if err := keyring.Set(s.service, account, token); err != nil {
		return fmt.Errorf("saving token to the Secret Service (is GNOME Keyring or KWallet running and unlocked?): %w", err)
	}
	return nil
}

func (s SecretService) Delete(account string) error {
	err := keyring.Delete(s.service, account)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("removing token from the Secret Service: %w", err)
	}
	return nil
}
