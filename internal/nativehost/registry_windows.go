package nativehost

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// SystemRegistry is the real Windows registry (HKEY_CURRENT_USER).
type SystemRegistry struct{}

// KeyExists reports whether HKEY_CURRENT_USER\key exists.
func (SystemRegistry) KeyExists(key string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	_ = k.Close()
	return true
}

// SetDefault sets the default value of HKEY_CURRENT_USER\key, creating it.
func (SystemRegistry) SetDefault(key, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("create key: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue("", value); err != nil {
		return fmt.Errorf("set value: %w", err)
	}
	return nil
}

// DefaultValue returns the default value of HKEY_CURRENT_USER\key.
func (SystemRegistry) DefaultValue(key string) (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("open key: %w", err)
	}
	defer k.Close()
	v, _, err := k.GetStringValue("")
	if err != nil {
		return "", fmt.Errorf("read value: %w", err)
	}
	return v, nil
}

// DeleteKey deletes HKEY_CURRENT_USER\key; a missing key is not an error.
func (SystemRegistry) DeleteKey(key string) error {
	err := registry.DeleteKey(registry.CURRENT_USER, key)
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("delete key: %w", err)
	}
	return nil
}
