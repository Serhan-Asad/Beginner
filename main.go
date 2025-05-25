package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type PullRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  string `json:"head"`
	Base  string `json:"base"`
}

func hasChanges() (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check git status: %v", err)
	}
	return len(output) > 0, nil
}

func createPullRequest(branchName string) error {
	// Get GitHub token from environment
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is not set")
	}

	// Get repository info
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	repoURL, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get repository URL: %v", err)
	}

	// Parse repository URL to get owner and repo name
	// Example URL: https://github.com/owner/repo.git
	parts := strings.Split(strings.TrimSpace(string(repoURL)), "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid repository URL format")
	}
	repoName := strings.TrimSuffix(parts[len(parts)-1], ".git")
	owner := parts[len(parts)-2]

	// Create pull request
	pr := PullRequest{
		Title: "Auto PR by test program",
		Body:  "This is an automated pull request created by the test program.",
		Head:  branchName,
		Base:  "main",
	}

	jsonData, err := json.Marshal(pr)
	if err != nil {
		return fmt.Errorf("failed to marshal PR data: %v", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", owner, repoName)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusCreated {
		var errorBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errorBody)
		return fmt.Errorf("failed to create PR: %s", errorBody["message"])
	}

	fmt.Println("Pull request created successfully!")
	return nil
}

func pushToGitHub() error {
	//are we in a git repository?
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("not in a git repository: %v", err)
	}

	// Get the current branch
	cmd = exec.Command("git", "branch", "--show-current")
	branch, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %v", err)
	}
	branchName := strings.TrimSpace(string(branch))
	fmt.Printf("Current branch: %s\n", branchName)

	// Check if there are any changes
	hasChanges, err := hasChanges()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %v", err)
	}

	if !hasChanges {
		fmt.Println("No changes to commit. Proceeding with push...")
		return nil
	} else {
		// Add all changes
		cmd = exec.Command("git", "add", ".")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to add changes: %v", err)
		}

		// Commit changes
		fmt.Println("Committing changes...")
		cmd = exec.Command("git", "commit", "-m", "Auto-commit by test program")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to commit changes: %v", err)
		}
		fmt.Println("Changes added to commit")
	}

	// Push to GitHub
	fmt.Println("Pushing to GitHub...")
	cmd = exec.Command("git", "push", "origin", branchName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to push to GitHub: %v", err)
	}

	// Only create pull request if not on main branch
	if branchName != "main" {
		fmt.Println("Creating pull request...")
		if err := createPullRequest(branchName); err != nil {
			return fmt.Errorf("failed to create pull request: %v", err)
		}
	} else {
		fmt.Println("On main branch - skipping pull request creation")
	}

	return nil
}

func main() {
	fmt.Println("Attempting to push to GitHub and create PR...")
	err := pushToGitHub()
	if err != nil {
		fmt.Printf("Failed to push to GitHub and create PR: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Done")
	
}
