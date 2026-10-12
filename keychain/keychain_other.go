//go:build !darwin

package keychain

import "errors"

type unsupported struct{}

// New returns a Store that always fails: only the macOS Keychain is
// supported for now.
func New(string) Store { return unsupported{} }

var errUnsupported = errors.New("token storage is only supported on macOS (Keychain)")

func (unsupported) Get(string) (string, error) { return "", errUnsupported }
func (unsupported) Set(string, string) error   { return errUnsupported }
func (unsupported) Delete(string) error        { return errUnsupported }
