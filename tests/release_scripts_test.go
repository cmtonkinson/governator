package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to determine repo root: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestReleaseScriptsHappyPath(t *testing.T) {

	root := repoRoot(t)
	tempDir := t.TempDir()
	script := filepath.Join(root, "scripts", "release.sh")

	args := []string{
		"--version", "1.2.3",
		"--commit", "deadbeef",
		"--built-at", "2025-01-01T00:00:00Z",
		"--out-dir", tempDir,
		"brew",
		"apt",
	}

	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("release script failed: %v\n%s", err, out)
	}

	brewArchive := filepath.Join(tempDir, "homebrew", "governator-1.2.3.tar.gz")
	if _, err := os.Stat(brewArchive); err != nil {
		t.Fatalf("missing brew artifact: %v", err)
	}

	aptDeb := filepath.Join(tempDir, "apt", "governator_1.2.3_amd64.deb")
	if _, err := os.Stat(aptDeb); err != nil {
		t.Fatalf("missing apt artifact: %v", err)
	}

	binary := extractBinaryFromDeb(t, aptDeb)
	assertBinaryContainsMetadata(t, binary)
}

func TestReleaseScriptsMissingVersion(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "release.sh")

	cmd := exec.Command("bash", script, "brew")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure when version is missing, got success")
	}
	if !strings.Contains(string(out), "--version <semver> is required") {
		t.Fatalf("unexpected error output: %s", out)
	}
}

func extractBinaryFromDeb(t *testing.T, deb string) string {
	t.Helper()
	tmp := t.TempDir()

	cmd := exec.Command("ar", "x", deb)
	cmd.Dir = tmp
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ar x failed: %v\n%s", err, out)
	}

	dataTar := filepath.Join(tmp, "data.tar.gz")
	cmd = exec.Command("tar", "-xzf", dataTar)
	cmd.Dir = tmp
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar -xzf failed: %v\n%s", err, out)
	}

	binary := filepath.Join(tmp, "usr", "local", "bin", "governator")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("extracted binary missing: %v", err)
	}
	return binary
}

func assertBinaryContainsMetadata(t *testing.T, binary string) {
	t.Helper()
	content, err := os.ReadFile(binary)
	if err != nil {
		t.Fatalf("read binary for inspection: %v", err)
	}
	for _, expected := range []string{
		"1.2.3",
		"deadbeef",
		"2025-01-01T00:00:00Z",
	} {
		if !bytes.Contains(content, []byte(expected)) {
			t.Fatalf("binary missing metadata %q", expected)
		}
	}
	if runtime.GOOS == "linux" {
		cmd := exec.Command(binary, "version")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("running extracted binary: %v\n%s", err, out)
		}
		expected := "version=1.2.3 commit=deadbeef built_at=2025-01-01T00:00:00Z"
		if strings.TrimSpace(string(out)) != expected {
			t.Fatalf("version output mismatch: %q", out)
		}
	}
}
