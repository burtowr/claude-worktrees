package worktree

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Manager handles git worktree operations
type Manager struct {
	repoRoot string
	state    *State
}

// NewManager creates a new worktree manager
func NewManager(repoRoot string) (*Manager, error) {
	// Verify this is a git repo
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository: %s", repoRoot)
	}

	state, err := LoadState(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}
	state.RepoRoot = repoRoot

	return &Manager{
		repoRoot: repoRoot,
		state:    state,
	}, nil
}

// generateID creates a unique agent ID
func generateID() string {
	timestamp := time.Now().Format("20060102")
	bytes := make([]byte, 2)
	rand.Read(bytes)
	suffix := hex.EncodeToString(bytes)
	return fmt.Sprintf("cwt-%s-%s", timestamp, suffix)
}

// slugify converts a task description to a safe branch suffix
func slugify(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)
	// Replace non-alphanumeric with dashes
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	s = reg.ReplaceAllString(s, "-")
	// Trim dashes
	s = strings.Trim(s, "-")
	// Limit length
	if len(s) > 30 {
		s = s[:30]
	}
	return s
}

// git runs a git command and returns output
func (m *Manager) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = m.repoRoot
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// GetCurrentBranch returns the current branch name
func (m *Manager) GetCurrentBranch() (string, error) {
	return m.git("rev-parse", "--abbrev-ref", "HEAD")
}

// GetCurrentCommit returns the current commit SHA
func (m *Manager) GetCurrentCommit() (string, error) {
	return m.git("rev-parse", "HEAD")
}

// CreateWorktree creates a new worktree for an agent
func (m *Manager) CreateWorktree(task string) (*Agent, error) {
	// Generate IDs
	id := generateID()
	slug := slugify(task)
	branch := fmt.Sprintf("cwt/%s/%s", id, slug)
	worktreePath := filepath.Join(m.repoRoot, m.state.WorktreeDir, id)

	// Get current branch and commit for base
	baseBranch, err := m.GetCurrentBranch()
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}

	baseCommit, err := m.GetCurrentCommit()
	if err != nil {
		return nil, fmt.Errorf("failed to get current commit: %w", err)
	}

	// Create worktree with new branch
	if _, err := m.git("worktree", "add", "-b", branch, worktreePath); err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	// Create agent record
	agent := &Agent{
		ID:         id,
		Branch:     branch,
		Worktree:   worktreePath,
		Task:       task,
		Status:     StatusRunning,
		BaseBranch: baseBranch,
		BaseCommit: baseCommit,
		CreatedAt:  time.Now(),
	}

	// Save state
	m.state.AddAgent(agent)
	if err := m.state.Save(); err != nil {
		// Try to clean up worktree on save failure
		m.git("worktree", "remove", worktreePath)
		m.git("branch", "-D", branch)
		return nil, fmt.Errorf("failed to save state: %w", err)
	}

	return agent, nil
}

// RemoveWorktree removes a worktree and its branch
func (m *Manager) RemoveWorktree(id string) error {
	agent, ok := m.state.GetAgent(id)
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}

	// Remove worktree
	if _, err := m.git("worktree", "remove", agent.Worktree); err != nil {
		// Try force remove if normal fails
		m.git("worktree", "remove", "--force", agent.Worktree)
	}

	// Delete branch
	if _, err := m.git("branch", "-d", agent.Branch); err != nil {
		// Force delete if not merged
		m.git("branch", "-D", agent.Branch)
	}

	// Update state
	m.state.RemoveAgent(id)
	return m.state.Save()
}

// GetAgent returns an agent by ID
func (m *Manager) GetAgent(id string) (*Agent, bool) {
	return m.state.GetAgent(id)
}

// ListAgents returns all agents
func (m *Manager) ListAgents() []*Agent {
	return m.state.ListAgents()
}

// UpdateAgentStatus updates an agent's status
func (m *Manager) UpdateAgentStatus(id string, status AgentStatus) error {
	agent, ok := m.state.GetAgent(id)
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}

	agent.Status = status
	return m.state.Save()
}

// GetWorktreePath returns the absolute path to the worktrees directory
func (m *Manager) GetWorktreePath() string {
	return filepath.Join(m.repoRoot, m.state.WorktreeDir)
}

// GetRepoRoot returns the repository root
func (m *Manager) GetRepoRoot() string {
	return m.repoRoot
}

// GetDiff returns the diff between agent branch and base
func (m *Manager) GetDiff(id string) (string, error) {
	agent, ok := m.state.GetAgent(id)
	if !ok {
		return "", fmt.Errorf("agent %s not found", id)
	}

	return m.git("diff", agent.BaseBranch+"..."+agent.Branch)
}

// GetCommits returns commits on agent branch since diverging from base
func (m *Manager) GetCommits(id string) (string, error) {
	agent, ok := m.state.GetAgent(id)
	if !ok {
		return "", fmt.Errorf("agent %s not found", id)
	}

	return m.git("log", "--oneline", agent.BaseBranch+".."+agent.Branch)
}

// HasConflicts checks if merging would cause conflicts
func (m *Manager) HasConflicts(id string) (bool, error) {
	agent, ok := m.state.GetAgent(id)
	if !ok {
		return false, fmt.Errorf("agent %s not found", id)
	}

	// Get merge base
	mergeBase, err := m.git("merge-base", agent.BaseBranch, agent.Branch)
	if err != nil {
		return false, err
	}

	// Check merge-tree for conflicts
	output, _ := m.git("merge-tree", mergeBase, agent.BaseBranch, agent.Branch)
	return strings.Contains(output, "<<<<<<"), nil
}

// Merge merges an agent's branch into the base branch
func (m *Manager) Merge(id string) error {
	agent, ok := m.state.GetAgent(id)
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}

	// Update status
	agent.Status = StatusMerging
	m.state.Save()

	// Checkout base branch
	if _, err := m.git("checkout", agent.BaseBranch); err != nil {
		return fmt.Errorf("failed to checkout base branch: %w", err)
	}

	// Merge
	msg := fmt.Sprintf("Merge %s: %s", agent.ID, agent.Task)
	if _, err := m.git("merge", "--no-ff", "-m", msg, agent.Branch); err != nil {
		return fmt.Errorf("merge failed: %w", err)
	}

	// Update status
	now := time.Now()
	agent.Status = StatusMerged
	agent.MergedAt = &now
	return m.state.Save()
}

// State returns the current state
func (m *Manager) State() *State {
	return m.state
}
