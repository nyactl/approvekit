package keychain

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestReplaceTokenOfAnotherBuild needs the login keychain and a Go
// toolchain, so it only runs when APPROVEKIT_KEYCHAIN_TEST=1.
func TestReplaceTokenOfAnotherBuild(t *testing.T) {
	if os.Getenv("APPROVEKIT_KEYCHAIN_TEST") != "1" {
		t.Skip("set APPROVEKIT_KEYCHAIN_TEST=1 to test against the login keychain")
	}
	bin := filepath.Join(t.TempDir(), "otherbuild")
	if out, err := exec.Command("go", "build", "-o", bin, "./testdata/otherbuild").CombinedOutput(); err != nil {
		t.Fatalf("building the other build: %v\n%s", err, out)
	}
	k := New("approvekit-test")
	key := AccountKey("https://keychain-owner-test.invalid", "test@example.com")
	t.Cleanup(func() { _ = k.Delete(key) })

	if out, err := exec.Command(bin, "approvekit-test", key, "theirs").CombinedOutput(); err != nil {
		t.Fatalf("other build: %v\n%s", err, out)
	}
	if err := k.Set(key, "mine"); err != nil {
		t.Fatal(err)
	}
	// The item is this binary's now, so reading it does not ask.
	if got, err := k.Get(key); err != nil || got != "mine" {
		t.Fatalf("Get = %q, %v", got, err)
	}
}
