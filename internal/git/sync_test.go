package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncInitializesAndResetsWorkingTree(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "knowledge.git")
	working := filepath.Join(t.TempDir(), "knowledge")
	runTestGit(t, "init", "--bare", remote)

	seed := filepath.Join(t.TempDir(), "seed")
	runTestGit(t, "init", seed)
	writeTestFile(t, filepath.Join(seed, "docs", "index.md"), "# First revision")
	runTestGit(t, "-C", seed, "add", ".")
	runTestGit(t, "-C", seed, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
	runTestGit(t, "-C", seed, "remote", "add", "origin", remote)
	runTestGit(t, "-C", seed, "push", "-u", "origin", "HEAD:main")

	pulled, message, err := Sync(remote, working, "main")
	if err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if !pulled || !strings.Contains(message, "main") {
		t.Fatalf("unexpected sync result: pulled=%v message=%q", pulled, message)
	}
	content, err := os.ReadFile(filepath.Join(working, "docs", "index.md"))
	if err != nil {
		t.Fatalf("read synced content: %v", err)
	}
	if string(content) != "# First revision" {
		t.Fatalf("unexpected synced content %q", content)
	}
}

func runTestGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
