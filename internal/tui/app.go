package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wburton/cwt/internal/pty"
	"github.com/wburton/cwt/internal/worktree"
)

// Tab represents a single tab in the TUI
type Tab struct {
	ID        string
	Name      string
	IsMain    bool
	SessionID string
	Agent     *worktree.Agent
}

// Model is the main bubbletea model
type Model struct {
	tabs         []Tab
	activeTab    int
	ptyManager   *pty.Manager
	wtManager    *worktree.Manager
	width        int
	height       int
	inputMode    bool
	inputBuffer  string
	inputPrompt  string
	inputAction  func(string)
	quitting     bool
	lastOutput   map[string]string // Cache last output per session
}

// OutputMsg is sent when PTY output is received
type OutputMsg struct {
	SessionID string
	Data      []byte
}

// NewModel creates a new TUI model
func NewModel(repoRoot string) (*Model, error) {
	wtManager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return nil, err
	}

	m := &Model{
		tabs:       []Tab{},
		activeTab:  0,
		ptyManager: pty.NewManager(),
		wtManager:  wtManager,
		lastOutput: make(map[string]string),
	}

	// Create main tab
	mainSession, err := m.ptyManager.Spawn("main", repoRoot, "Main orchestrator")
	if err != nil {
		return nil, fmt.Errorf("failed to spawn main session: %w", err)
	}

	// Set up output callback
	mainSession.SetOutputCallback(func(data []byte) {
		// This will trigger a view update
	})

	m.tabs = append(m.tabs, Tab{
		ID:        "main",
		Name:      "Main",
		IsMain:    true,
		SessionID: "main",
	})

	// Restore existing agents
	for _, agent := range wtManager.ListAgents() {
		if agent.Status == worktree.StatusRunning {
			session, err := m.ptyManager.Spawn(agent.ID, agent.Worktree, agent.Task)
			if err != nil {
				continue // Skip failed sessions
			}
			session.SetOutputCallback(func(data []byte) {})

			m.tabs = append(m.tabs, Tab{
				ID:        agent.ID,
				Name:      truncate(agent.Task, 15),
				SessionID: agent.ID,
				Agent:     agent,
			})
		}
	}

	return m, nil
}

// truncate shortens a string to max length
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// Init implements bubbletea.Model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		tickCmd(),
	)
}

// tickCmd creates a tick command for periodic updates
func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(_ time.Time) tea.Msg {
		return tickMsg{}
	})
}

type tickMsg struct{}

// Update implements bubbletea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Resize all PTY sessions
		m.ptyManager.ResizeAll(uint16(m.height-3), uint16(m.width))
		return m, nil

	case tickMsg:
		// Update output caches
		for _, tab := range m.tabs {
			if session, ok := m.ptyManager.Get(tab.SessionID); ok {
				m.lastOutput[tab.SessionID] = session.Output()
			}
		}
		return m, tickCmd()

	case OutputMsg:
		// Output received, view will update
		return m, nil
	}

	return m, nil
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle input mode
	if m.inputMode {
		return m.handleInputMode(msg)
	}

	switch msg.String() {
	// Cmd+Left (Option+Left on macOS terminal) - previous tab
	case "alt+left", "shift+left":
		if m.activeTab > 0 {
			m.activeTab--
		}
		return m, nil

	// Cmd+Right - next tab
	case "alt+right", "shift+right":
		if m.activeTab < len(m.tabs)-1 {
			m.activeTab++
		}
		return m, nil

	// Cmd+N - new agent
	case "alt+n", "ctrl+n":
		m.inputMode = true
		m.inputPrompt = "Task description: "
		m.inputBuffer = ""
		m.inputAction = m.createNewAgent
		return m, nil

	// Cmd+W - close tab
	case "alt+w", "ctrl+w":
		if m.activeTab > 0 { // Don't close main tab
			return m.closeCurrentTab()
		}
		return m, nil

	// Cmd+M - merge current tab
	case "alt+m", "ctrl+m":
		if m.activeTab > 0 {
			return m.mergeCurrentTab()
		}
		return m, nil

	// Cmd+Q - quit
	case "alt+q", "ctrl+q", "ctrl+c":
		m.quitting = true
		m.ptyManager.StopAll()
		return m, tea.Quit

	// Forward other keys to active session
	default:
		if tab := m.currentTab(); tab != nil {
			if session, ok := m.ptyManager.Get(tab.SessionID); ok {
				session.Write([]byte(msg.String()))
			}
		}
		return m, nil
	}
}

func (m Model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if m.inputAction != nil && m.inputBuffer != "" {
			m.inputAction(m.inputBuffer)
		}
		m.inputMode = false
		m.inputBuffer = ""
		return m, nil

	case tea.KeyEsc:
		m.inputMode = false
		m.inputBuffer = ""
		return m, nil

	case tea.KeyBackspace:
		if len(m.inputBuffer) > 0 {
			m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
		}
		return m, nil

	default:
		if msg.Type == tea.KeyRunes {
			m.inputBuffer += string(msg.Runes)
		}
		return m, nil
	}
}

func (m *Model) createNewAgent(task string) {
	agent, err := m.wtManager.CreateWorktree(task)
	if err != nil {
		// TODO: Show error to user
		return
	}

	session, err := m.ptyManager.Spawn(agent.ID, agent.Worktree, agent.Task)
	if err != nil {
		// Clean up worktree on failure
		m.wtManager.RemoveWorktree(agent.ID)
		return
	}
	session.SetOutputCallback(func(data []byte) {})

	m.tabs = append(m.tabs, Tab{
		ID:        agent.ID,
		Name:      truncate(task, 15),
		SessionID: agent.ID,
		Agent:     agent,
	})
	m.activeTab = len(m.tabs) - 1
}

func (m Model) closeCurrentTab() (tea.Model, tea.Cmd) {
	if m.activeTab == 0 {
		return m, nil // Can't close main tab
	}

	tab := m.tabs[m.activeTab]

	// Stop PTY session
	m.ptyManager.Kill(tab.SessionID)

	// Remove worktree if agent
	if tab.Agent != nil {
		m.wtManager.RemoveWorktree(tab.Agent.ID)
	}

	// Remove tab
	m.tabs = append(m.tabs[:m.activeTab], m.tabs[m.activeTab+1:]...)
	if m.activeTab >= len(m.tabs) {
		m.activeTab = len(m.tabs) - 1
	}

	return m, nil
}

func (m Model) mergeCurrentTab() (tea.Model, tea.Cmd) {
	if m.activeTab == 0 {
		return m, nil
	}

	tab := m.tabs[m.activeTab]
	if tab.Agent == nil {
		return m, nil
	}

	// TODO: Invoke merge orchestrator instead of direct merge
	if err := m.wtManager.Merge(tab.Agent.ID); err != nil {
		// TODO: Show error
		return m, nil
	}

	// Close the tab after merge
	return m.closeCurrentTab()
}

func (m Model) currentTab() *Tab {
	if m.activeTab >= 0 && m.activeTab < len(m.tabs) {
		return &m.tabs[m.activeTab]
	}
	return nil
}

// View implements bubbletea.Model
func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	var s strings.Builder

	// Tab bar
	s.WriteString(m.renderTabBar())
	s.WriteString("\n")

	// Session output
	s.WriteString(m.renderSession())

	// Status bar / Input
	s.WriteString(m.renderStatusBar())

	return s.String()
}

func (m Model) renderTabBar() string {
	var tabs []string

	activeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("62")).
		Padding(0, 1)

	inactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("238")).
		Padding(0, 1)

	for i, tab := range m.tabs {
		name := tab.Name
		if tab.IsMain {
			name = "● " + name
		}

		if i == m.activeTab {
			tabs = append(tabs, activeStyle.Render(name))
		} else {
			tabs = append(tabs, inactiveStyle.Render(name))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m Model) renderSession() string {
	height := m.height - 3 // Account for tab bar and status bar
	if height < 1 {
		height = 1
	}

	tab := m.currentTab()
	if tab == nil {
		return ""
	}

	output := m.lastOutput[tab.SessionID]

	// Get last N lines that fit the screen
	lines := strings.Split(output, "\n")
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}

	// Pad to fill height
	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n") + "\n"
}

func (m Model) renderStatusBar() string {
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("236")).
		Width(m.width)

	if m.inputMode {
		return statusStyle.Render(m.inputPrompt + m.inputBuffer + "█")
	}

	// Show keybinds
	help := "  ⌥← Prev Tab │ ⌥→ Next Tab │ ⌥N New │ ⌥M Merge │ ⌥W Close │ ⌥Q Quit"
	return statusStyle.Render(help)
}
