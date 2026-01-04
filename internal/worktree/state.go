package worktree

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// AgentStatus represents the current state of an agent
type AgentStatus string

const (
	StatusPending   AgentStatus = "pending"
	StatusRunning   AgentStatus = "running"
	StatusCompleted AgentStatus = "completed"
	StatusMerging   AgentStatus = "merging"
	StatusMerged    AgentStatus = "merged"
	StatusFailed    AgentStatus = "failed"
)

// Agent represents a worktree agent
type Agent struct {
	ID         string      `json:"id"`
	Branch     string      `json:"branch"`
	Worktree   string      `json:"worktree"`
	Task       string      `json:"task"`
	Status     AgentStatus `json:"status"`
	BaseBranch string      `json:"baseBranch"`
	BaseCommit string      `json:"baseCommit"`
	CreatedAt  time.Time   `json:"createdAt"`
	MergedAt   *time.Time  `json:"mergedAt,omitempty"`
}

// State represents the persisted state of all agents
type State struct {
	Version      string            `json:"version"`
	RepoRoot     string            `json:"repoRoot"`
	WorktreeDir  string            `json:"worktreeDir"`
	Agents       map[string]*Agent `json:"agents"`
	MergeHistory []MergeRecord     `json:"mergeHistory"`
}

// MergeRecord tracks a completed merge
type MergeRecord struct {
	AgentID           string    `json:"agentId"`
	MergedAt          time.Time `json:"mergedAt"`
	MergeCommit       string    `json:"mergeCommit"`
	ConflictsResolved int       `json:"conflictsResolved"`
	ConflictsEscalated int      `json:"conflictsEscalated"`
}

// NewState creates a new empty state
func NewState(repoRoot string) *State {
	return &State{
		Version:      "1.0",
		RepoRoot:     repoRoot,
		WorktreeDir:  ".worktrees",
		Agents:       make(map[string]*Agent),
		MergeHistory: []MergeRecord{},
	}
}

// StateFilePath returns the path to the state file for a repo
func StateFilePath(repoRoot string) string {
	return filepath.Join(repoRoot, ".cwt", "state.json")
}

// LoadState loads state from disk, creating a new one if it doesn't exist
func LoadState(repoRoot string) (*State, error) {
	stateFile := StateFilePath(repoRoot)

	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return NewState(repoRoot), nil
		}
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	// Ensure maps are initialized
	if state.Agents == nil {
		state.Agents = make(map[string]*Agent)
	}

	return &state, nil
}

// Save persists the state to disk
func (s *State) Save() error {
	stateFile := StateFilePath(s.RepoRoot)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(stateFile, data, 0644)
}

// AddAgent adds a new agent to the state
func (s *State) AddAgent(agent *Agent) {
	s.Agents[agent.ID] = agent
}

// GetAgent returns an agent by ID
func (s *State) GetAgent(id string) (*Agent, bool) {
	agent, ok := s.Agents[id]
	return agent, ok
}

// RemoveAgent removes an agent from the state
func (s *State) RemoveAgent(id string) {
	delete(s.Agents, id)
}

// ListAgents returns all agents
func (s *State) ListAgents() []*Agent {
	agents := make([]*Agent, 0, len(s.Agents))
	for _, agent := range s.Agents {
		agents = append(agents, agent)
	}
	return agents
}

// ListByStatus returns agents with a specific status
func (s *State) ListByStatus(status AgentStatus) []*Agent {
	var agents []*Agent
	for _, agent := range s.Agents {
		if agent.Status == status {
			agents = append(agents, agent)
		}
	}
	return agents
}
