package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Sync(repoURL, repoPath string) (bool, string, error) {
	gitDir := filepath.Join(repoPath, ".git")
	_, err := os.Stat(gitDir)

	if os.IsNotExist(err) {
		// Not a git repository, try to initialize it
		if repoURL == "" {
			return false, "No KNOWLEDGE_REPO_URL configured and directory is not a git repository", nil
		}
		
		// Run init sequence
		initCommands := [][]string{
			{"git", "-C", repoPath, "init"},
			{"git", "-C", repoPath, "remote", "add", "origin", repoURL},
			{"git", "-C", repoPath, "fetch", "origin"},
			{"git", "-C", repoPath, "reset", "--hard", "origin/main"},
		}
		
		var lastOutput string
		for _, args := range initCommands {
			cmd := exec.Command(args[0], args[1:]...)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				return false, strings.TrimSpace(stderr.String()), fmt.Errorf("git %s failed: %w", args[3], err)
			}
			lastOutput = strings.TrimSpace(stdout.String())
			if lastOutput == "" {
				lastOutput = strings.TrimSpace(stderr.String())
			}
		}
		
		// Optionally branch setup, but reset --hard origin/main leaves us in detached HEAD or on main if it existed.
		// Let's ensure we're on main tracking origin/main.
		exec.Command("git", "-C", repoPath, "branch", "-M", "main").Run()
		exec.Command("git", "-C", repoPath, "branch", "--set-upstream-to=origin/main", "main").Run()
		
		return true, lastOutput, nil
	} else if err != nil {
		return false, "", fmt.Errorf("failed to stat .git directory: %w", err)
	}

	// It is a git repository, run fetch and reset to force sync
	fetchCmd := exec.Command("git", "-C", repoPath, "fetch", "origin")
	if err := fetchCmd.Run(); err != nil {
		return false, "git fetch failed", fmt.Errorf("git fetch failed: %w", err)
	}

	resetCmd := exec.Command("git", "-C", repoPath, "reset", "--hard", "origin/main")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	resetCmd.Stdout = &stdout
	resetCmd.Stderr = &stderr
	if err := resetCmd.Run(); err != nil {
		return false, strings.TrimSpace(stderr.String()), fmt.Errorf("git reset failed: %w", err)
	}
	
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = strings.TrimSpace(stderr.String())
	}
	return true, output, nil
}
