package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/google/go-github/v45/github"
	"golang.org/x/oauth2"
)

// GitHubService handles all GitHub operations
type GitHubService struct {
	client *github.Client
	ctx    context.Context
	owner  string
	repo   string
}

const prFile = ".last_pr"

// NewGitHubService creates a new GitHub service instance
func NewGitHubService() (*GitHubService, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable not set")
	}

	// Get repository info
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	repoURL, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get repository URL: %v", err)
	}

	// Parse repository URL to get owner and repo name
	parts := strings.Split(strings.TrimSpace(string(repoURL)), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid repository URL format")
	}
	repoName := strings.TrimSuffix(parts[len(parts)-1], ".git")
	owner := parts[len(parts)-2]

	// Initialize GitHub client
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	return &GitHubService{
		client: client,
		ctx:    ctx,
		owner:  owner,
		repo:   repoName,
	}, nil
}

// PushChanges pushes changes to GitHub and creates a PR if not on main branch
func (s *GitHubService) PushChanges() error {
	// Check if we're in a git repository
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("not in a git repository: %v", err)
	}

	// Get current branch
	cmd = exec.Command("git", "branch", "--show-current")
	branch, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %v", err)
	}
	branchName := strings.TrimSpace(string(branch))

	// Check for changes
	hasChanges, err := s.hasChanges()
	if err != nil {
		return err
	}

	if !hasChanges {
		fmt.Println("No changes to commit. Skipping push.")
		return nil
	}

	// Add and commit changes
	if err := s.commitChanges(); err != nil {
		return err
	}

	// Push to GitHub
	fmt.Println("Pushing to GitHub...")
	cmd = exec.Command("git", "push", "origin", branchName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to push to GitHub: %v", err)
	}

	// Create PR if not on main branch
	if branchName != "main" {
		return s.createPullRequest(branchName)
	}

	fmt.Println("On main branch - skipping pull request creation")
	return nil
}

// getLatestPRForBranch gets the most recent PR for the current branch
func (s *GitHubService) getLatestPRForBranch(branchName string) (*github.PullRequest, error) {
	// List PRs for the current branch
	prs, _, err := s.client.PullRequests.List(s.ctx, s.owner, s.repo, &github.PullRequestListOptions{
		Head:  fmt.Sprintf("%s:%s", s.owner, branchName),
		State: "open",
	})
	if err != nil {
		return nil, fmt.Errorf("error listing PRs: %v", err)
	}

	if len(prs) == 0 {
		return nil, fmt.Errorf("no open PRs found for branch %s", branchName)
	}

	// Return the most recent PR (first in the list)
	return prs[0], nil
}

// ReviewPR reviews a pull request and returns a review comment
func (s *GitHubService) ReviewPR(prNumber int) (string, error) {
	// If no PR number provided, try to get it from the stored file
	if prNumber == 0 {
		prURL, err := os.ReadFile(prFile)
		if err != nil {
			return "", fmt.Errorf("no PR number provided and no stored PR found: %v", err)
		}

		// Parse PR number from URL
		parts := strings.Split(string(prURL), "/")
		if len(parts) < 2 {
			return "", fmt.Errorf("invalid PR URL format")
		}
		prNumber, err = strconv.Atoi(parts[len(parts)-1])
		if err != nil {
			return "", fmt.Errorf("invalid PR number in URL: %v", err)
		}
		fmt.Printf("Using stored PR #%d\n", prNumber)
	}

	pr, _, err := s.client.PullRequests.Get(s.ctx, s.owner, s.repo, prNumber)
	if err != nil {
		return "", fmt.Errorf("error getting PR: %v", err)
	}

	files, _, err := s.client.PullRequests.ListFiles(s.ctx, s.owner, s.repo, prNumber, nil)
	if err != nil {
		return "", fmt.Errorf("error getting PR files: %v", err)
	}

	var reviewContent strings.Builder
	reviewContent.WriteString(fmt.Sprintf("PR #%d: %s\n\n", prNumber, *pr.Title))

	for _, file := range files {
		reviewContent.WriteString(fmt.Sprintf("File: %s\n", *file.Filename))
		if file.Patch != nil {
			reviewContent.WriteString(*file.Patch)
		}
		reviewContent.WriteString("\n" + strings.Repeat("-", 80) + "\n")
	}

	fmt.Println(reviewContent.String())
	return reviewContent.String(), nil
}

// Helper functions
func (s *GitHubService) hasChanges() (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check git status: %v", err)
	}
	return len(output) > 0, nil
}

func (s *GitHubService) commitChanges() error {
	// Add all changes
	cmd := exec.Command("git", "add", ".")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add changes: %v", err)
	}

	// Commit changes
	fmt.Println("Committing changes...")
	cmd = exec.Command("git", "commit", "-m", "Auto-commit by GitHub service")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}
	fmt.Println("Changes committed successfully")
	return nil
}

func (s *GitHubService) createPullRequest(branchName string) error {
	pr := &github.NewPullRequest{
		Title: github.String("Auto PR by GitHub service"),
		Body:  github.String("This is an automated pull request created by the GitHub service."),
		Head:  github.String(branchName),
		Base:  github.String("main"),
	}

	newPR, _, err := s.client.PullRequests.Create(s.ctx, s.owner, s.repo, pr)
	if err != nil {
		return fmt.Errorf("failed to create pull request: %v", err)
	}

	// Store PR URL in file
	prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", s.owner, s.repo, *newPR.Number)
	err = os.WriteFile(prFile, []byte(prURL), 0644)
	if err != nil {
		return fmt.Errorf("failed to store PR URL: %v", err)
	}

	fmt.Printf("Pull request created successfully! PR #%d\n", *newPR.Number)
	return nil
}

func main() {
	githubService, err := NewGitHubService()
	if err != nil {
		fmt.Printf("Error creating GitHub service: %v\n", err)
		os.Exit(1)
	}

	// Push changes and create PR
	err = githubService.PushChanges()
	if err != nil {
		fmt.Printf("Error pushing changes: %v\n", err)
		os.Exit(1)
	}

	// Review the PR that was just created
	_, err = githubService.ReviewPR(0) // 0 means use the stored PR
	if err != nil {
		fmt.Printf("Error reviewing PR: %v\n", err)
		os.Exit(1)
	}
}
