package nativehost

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestSystemRegistry(t *testing.T) {
	t.Parallel()
	base := fmt.Sprintf(`Software\portcheck-test-%d`, os.Getpid())
	key := base + `\NativeMessagingHosts\` + HostName
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, key)
		_ = registry.DeleteKey(registry.CURRENT_USER, base+`\NativeMessagingHosts`)
		_ = registry.DeleteKey(registry.CURRENT_USER, base)
	})
	var r SystemRegistry
	if r.KeyExists(key) {
		t.Fatal("test key already exists")
	}
	if _, err := r.DefaultValue(key); err == nil {
		t.Error("reading a missing key succeeded")
	}
	if err := r.SetDefault(key, `C:\m.json`); err != nil {
		t.Fatal(err)
	}
	if v, err := r.DefaultValue(key); err != nil || v != `C:\m.json` || !r.KeyExists(key) {
		t.Errorf("after set: %q, %v", v, err)
	}
	// A key without a default value.
	if _, err := r.DefaultValue(base); err == nil {
		t.Error("reading a missing default value succeeded")
	}
	// A key with subkeys can't be deleted.
	if err := r.DeleteKey(base); err == nil {
		t.Error("deleting a key with subkeys succeeded")
	}
	if err := r.DeleteKey(key); err != nil || r.KeyExists(key) {
		t.Errorf("delete: %v", err)
	}
	if err := r.DeleteKey(key); err != nil {
		t.Errorf("deleting a missing key: %v", err)
	}
	if err := r.SetDefault(`Software\`+strings.Repeat("x", 300), "v"); err == nil {
		t.Error("creating an over-long key succeeded")
	}
}
