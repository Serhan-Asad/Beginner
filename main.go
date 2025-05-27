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

const prFile = ".last_pr"

// GitHubService handles all GitHub operations
type GitHubService struct {
	client *github.Client
	ctx    context.Context
	owner  string
	repo   string
}

// NewGitHubService creates a new GitHub service instance
func NewGitHubService() (*GitHubService, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable not set")
	}

	repoURL, err := exec.Command("git", "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get repository URL: %v", err)
	}

	parts := strings.Split(strings.TrimSpace(string(repoURL)), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid repository URL format")
	}
	repoName := strings.TrimSuffix(parts[len(parts)-1], ".git")
	owner := parts[len(parts)-2]

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)

	return &GitHubService{
		client: github.NewClient(tc),
		ctx:    ctx,
		owner:  owner,
		repo:   repoName,
	}, nil
}

// PushChanges pushes changes to GitHub and creates a PR if not on main branch
func (s *GitHubService) PushChanges() error {
	if err := s.validateGitRepo(); err != nil {
		return err
	}

	branchName, err := s.getCurrentBranch()
	if err != nil {
		return err
	}

	hasChanges, err := s.hasChanges()
	if err != nil {
		return err
	}

	if !hasChanges {
		fmt.Println("No changes to commit. Skipping push.")
		return nil
	}

	if err := s.commitChanges(); err != nil {
		return err
	}

	if err := s.pushToRemote(branchName); err != nil {
		return err
	}

	if branchName != "main" {
		return s.createPullRequest(branchName)
	}

	fmt.Println("On main branch - skipping pull request creation")
	return nil
}

// ReviewPR reviews a pull request and returns a review comment
func (s *GitHubService) ReviewPR(prNumber int) (string, error) {
	if prNumber == 0 {
		var err error
		prNumber, err = s.getStoredPRNumber()
		if err != nil {
			return "", err
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
func (s *GitHubService) validateGitRepo() error {
	if err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return fmt.Errorf("not in a git repository: %v", err)
	}
	return nil
}

func (s *GitHubService) getCurrentBranch() (string, error) {
	branch, err := exec.Command("git", "branch", "--show-current").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %v", err)
	}
	return strings.TrimSpace(string(branch)), nil
}

func (s *GitHubService) hasChanges() (bool, error) {
	output, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("failed to check git status: %v", err)
	}
	return len(output) > 0, nil
}

func (s *GitHubService) commitChanges() error {
	if err := exec.Command("git", "add", ".").Run(); err != nil {
		return fmt.Errorf("failed to add changes: %v", err)
	}

	fmt.Println("Committing changes...")
	if err := exec.Command("git", "commit", "-m", "Auto-commit by GitHub service").Run(); err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}
	fmt.Println("Changes committed successfully")
	return nil
}

func (s *GitHubService) pushToRemote(branchName string) error {
	fmt.Println("Pushing to GitHub...")
	if err := exec.Command("git", "push", "origin", branchName).Run(); err != nil {
		return fmt.Errorf("failed to push to GitHub: %v", err)
	}
	return nil
}

func (s *GitHubService) getStoredPRNumber() (int, error) {
	prURL, err := os.ReadFile(prFile)
	if err != nil {
		return 0, fmt.Errorf("no PR number provided and no stored PR found: %v", err)
	}

	parts := strings.Split(string(prURL), "/")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid PR URL format")
	}

	prNumber, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, fmt.Errorf("invalid PR number in URL: %v", err)
	}
	return prNumber, nil
}

func (s *GitHubService) createPullRequest(branchName string) error {
	prs, _, err := s.client.PullRequests.List(s.ctx, s.owner, s.repo, &github.PullRequestListOptions{
		Head:  fmt.Sprintf("%s:%s", s.owner, branchName),
		State: "open",
	})
	if err != nil {
		return fmt.Errorf("error checking existing PRs: %v", err)
	}

	var pr *github.PullRequest
	if len(prs) > 0 {
		pr = prs[0]
		fmt.Printf("Using existing PR #%d\n", *pr.Number)
	} else {
		newPR := &github.NewPullRequest{
			Title: github.String("Auto PR by GitHub service"),
			Body:  github.String("This is an automated pull request created by the GitHub service."),
			Head:  github.String(branchName),
			Base:  github.String("main"),
		}

		pr, _, err = s.client.PullRequests.Create(s.ctx, s.owner, s.repo, newPR)
		if err != nil {
			return fmt.Errorf("failed to create pull request: %v", err)
		}
		fmt.Printf("Created new PR #%d\n", *pr.Number)
	}

	prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", s.owner, s.repo, *pr.Number)
	if err := os.WriteFile(prFile, []byte(prURL), 0644); err != nil {
		return fmt.Errorf("failed to store PR URL: %v", err)
	}

	return nil
}

func main() {
	githubService, err := NewGitHubService()
	if err != nil {
		fmt.Printf("Error creating GitHub service: %v\n", err)
		os.Exit(1)
	}

	if err := githubService.PushChanges(); err != nil {
		fmt.Printf("Error pushing changes: %v\n", err)
		os.Exit(1)
	}

	if _, err := githubService.ReviewPR(0); err != nil {
		fmt.Printf("Error reviewing PR: %v\n", err)
		os.Exit(1)
	}
}
