package manage

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The validation runner may supply the exact release artifact it just built.
// Without it, ordinary source checkouts keep building their own engine.
func testEngineBinary(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("BACKPACK_CURRENT_BINARY"); bin != "" {
		info, err := os.Stat(bin)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("invalid BACKPACK_CURRENT_BINARY: %v", err)
		}
		return bin
	}
	bin := filepath.Join(t.TempDir(), "ravan-current")
	build := exec.Command("go", "build", "-o", bin, "../..")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("building the engine: %v", err)
	}
	return bin
}
