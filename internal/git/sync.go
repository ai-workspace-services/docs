package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Sync(repoURL, repoPath, ref string) (bool, string, error) {
	if strings.TrimSpace(repoPath) == "" {
		return false, "", fmt.Errorf("knowledge repository path is required")
	}
	if strings.TrimSpace(ref) == "" {
		ref = "main"
	}

	gitDir := filepath.Join(repoPath, ".git")
	_, err := os.Stat(gitDir)

	if os.IsNotExist(err) {
		// Not a git repository, initialize it only when an upstream is configured.
		if repoURL == "" {
			return false, "No KNOWLEDGE_REPO_URL configured and directory is not a git repository", nil
		}
		if err := os.MkdirAll(repoPath, 0o755); err != nil {
			return false, "", fmt.Errorf("create knowledge repository path: %w", err)
		}
		if _, err := runGit(repoPath, "init"); err != nil {
			return false, "", err
		}
		if _, err := runGit(repoPath, "remote", "add", "origin", repoURL); err != nil {
			return false, "", err
		}
	} else if err != nil {
		return false, "", fmt.Errorf("failed to stat .git directory: %w", err)
	}

	if repoURL != "" {
		remoteURL, remoteErr := runGit(repoPath, "remote", "get-url", "origin")
		if remoteErr != nil {
			if _, err := runGit(repoPath, "remote", "add", "origin", repoURL); err != nil {
				return false, "", err
			}
		} else if strings.TrimSpace(remoteURL) != strings.TrimSpace(repoURL) {
			if _, err := runGit(repoPath, "remote", "set-url", "origin", repoURL); err != nil {
				return false, "", err
			}
		}
	}

	if _, err := runGit(repoPath, "fetch", "--prune", "origin", ref); err != nil {
		return false, "git fetch failed", err
	}
	if _, err := runGit(repoPath, "reset", "--hard", "FETCH_HEAD"); err != nil {
		return false, "git reset failed", err
	}
	return true, fmt.Sprintf("synced knowledge repository to %s", ref), nil
}

func runGit(repoPath string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)), fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output)), nil
}
