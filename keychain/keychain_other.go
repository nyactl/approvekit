//go:build !darwin && !linux

package keychain

import "errors"

type unsupported struct{}

// New returns a Store that always fails: only the macOS Keychain and the
// Secret Service on Linux are supported.
func New(string) Store { return unsupported{} }

var errUnsupported = errors.New("token storage is only supported on macOS (Keychain) and Linux (Secret Service)")

func (unsupported) Get(string) (string, error) { return "", errUnsupported }
func (unsupported) Set(string, string) error   { return errUnsupported }
func (unsupported) Delete(string) error        { return errUnsupported }
