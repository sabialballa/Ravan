package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineRavanInstallRequiresSuccessfulChecksumVerification(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	raw, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	begin, end := strings.Index(source, "install_release() {"), strings.Index(source, "install_binary_from_tar() {")
	if begin < 0 || end <= begin {
		t.Fatal("installer function boundary not found")
	}
	for _, tc := range []struct {
		name       string
		manifest   bool
		verifyCode int
		success    bool
	}{
		{"missing manifest", false, 0, false}, {"missing asset entry", true, 1, false}, {"wrong hash", true, 2, false}, {"verified archive", true, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			input := filepath.Join(root, "input")
			output := filepath.Join(root, "output")
			for _, dir := range []string{input, output} {
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(input, "ravan_linux_amd64.tar.gz"), []byte("test archive"), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.manifest {
				if err := os.WriteFile(filepath.Join(input, "SHA256SUMS"), []byte("test manifest"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			block := "set -eu\nSCRIPT_DIR=\"$1\"\nINSTALL_DIR=\"$2\"\nASSET=ravan_linux_amd64.tar.gz\nVERIFY_CODE=\"$3\"\ninfo(){ :; }; warn(){ :; }; err(){ echo \"$*\" >&2; }; trusted_dir(){ return 0; }; verify_asset(){ return \"$VERIFY_CODE\"; }; fetch(){ exit 55; }\n" + source[begin:end] + "\ninstall_release\n"
			code := "0"
			if tc.verifyCode == 1 {
				code = "1"
			}
			if tc.verifyCode == 2 {
				code = "2"
			}
			cmd := exec.Command("bash", "-c", block, "install-test", input, output, code)
			err := cmd.Run()
			if (err == nil) != tc.success {
				t.Fatalf("install result %v; expected success=%v", err, tc.success)
			}
		})
	}
}
