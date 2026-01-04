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
	tabs        []Tab
	activeTab   int
	ptyManager  *pty.Manager
	wtManager   *worktree.Manager
	width       int
	height      int
	inputMode   bool
	inputBuffer string
	inputPrompt string
	inputAction func(string)
	quitting    bool
	ready       bool
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
		width:      80,
		height:     24,
	}

	// Create main tab
	_, err = m.ptyManager.Spawn("main", repoRoot, "Main orchestrator")
	if err != nil {
		return nil, fmt.Errorf("failed to spawn main session: %w", err)
	}

	m.tabs = append(m.tabs, Tab{
		ID:        "main",
		Name:      "Main",
		IsMain:    true,
		SessionID: "main",
	})

	// Restore existing agents
	for _, agent := range wtManager.ListAgents() {
		if agent.Status == worktree.StatusRunning {
			_, err := m.ptyManager.Spawn(agent.ID, agent.Worktree, agent.Task)
			if err != nil {
				continue // Skip failed sessions
			}

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
		tickCmd(),
	)
}

// tickCmd creates a tick command for periodic updates
func tickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(_ time.Time) tea.Msg {
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
		m.ready = true
		// Resize all PTY sessions (account for tab bar and status bar)
		termHeight := m.height - 2
		if termHeight < 1 {
			termHeight = 1
		}
		m.ptyManager.ResizeAll(termHeight, m.width)
		return m, nil

	case tickMsg:
		// Just trigger redraw
		return m, tickCmd()
	}

	return m, nil
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle input mode first
	if m.inputMode {
		return m.handleInputMode(msg)
	}

	// Check for our control keys
	keyStr := msg.String()

	switch keyStr {
	// Alt+Left - previous tab
	case "alt+left", "alt+[1;3D":
		if m.activeTab > 0 {
			m.activeTab--
		}
		return m, nil

	// Alt+Right - next tab
	case "alt+right", "alt+[1;3C":
		if m.activeTab < len(m.tabs)-1 {
			m.activeTab++
		}
		return m, nil

	// Alt+N - new agent
	case "alt+n":
		m.inputMode = true
		m.inputPrompt = "Task description: "
		m.inputBuffer = ""
		m.inputAction = m.createNewAgent
		return m, nil

	// Alt+W - close tab
	case "alt+w":
		if m.activeTab > 0 { // Don't close main tab
			return m.closeCurrentTab()
		}
		return m, nil

	// Alt+M - merge current tab
	case "alt+m":
		if m.activeTab > 0 {
			return m.mergeCurrentTab()
		}
		return m, nil

	// Alt+Q or Ctrl+C - quit
	case "alt+q", "ctrl+c":
		m.quitting = true
		m.ptyManager.StopAll()
		return m, tea.Quit
	}

	// Forward to active session
	if tab := m.currentTab(); tab != nil {
		if session, ok := m.ptyManager.Get(tab.SessionID); ok {
			// Convert key message to bytes
			data := keyToBytes(msg)
			if len(data) > 0 {
				session.Write(data)
			}
		}
	}

	return m, nil
}

// keyToBytes converts a tea.KeyMsg to the bytes that should be sent to PTY
func keyToBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyRunes:
		return []byte(string(msg.Runes))
	case tea.KeyEnter:
		return []byte{'\r'}
	case tea.KeyBackspace:
		return []byte{127}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeySpace:
		return []byte{' '}
	case tea.KeyEscape:
		return []byte{27}
	case tea.KeyUp:
		return []byte{27, '[', 'A'}
	case tea.KeyDown:
		return []byte{27, '[', 'B'}
	case tea.KeyRight:
		return []byte{27, '[', 'C'}
	case tea.KeyLeft:
		return []byte{27, '[', 'D'}
	case tea.KeyHome:
		return []byte{27, '[', 'H'}
	case tea.KeyEnd:
		return []byte{27, '[', 'F'}
	case tea.KeyPgUp:
		return []byte{27, '[', '5', '~'}
	case tea.KeyPgDown:
		return []byte{27, '[', '6', '~'}
	case tea.KeyDelete:
		return []byte{27, '[', '3', '~'}
	case tea.KeyCtrlA:
		return []byte{1}
	case tea.KeyCtrlB:
		return []byte{2}
	case tea.KeyCtrlC:
		return []byte{3}
	case tea.KeyCtrlD:
		return []byte{4}
	case tea.KeyCtrlE:
		return []byte{5}
	case tea.KeyCtrlF:
		return []byte{6}
	case tea.KeyCtrlG:
		return []byte{7}
	case tea.KeyCtrlH:
		return []byte{8}
	// KeyCtrlI is same as KeyTab, handled above
	case tea.KeyCtrlJ:
		return []byte{10}
	case tea.KeyCtrlK:
		return []byte{11}
	case tea.KeyCtrlL:
		return []byte{12}
	case tea.KeyCtrlN:
		return []byte{14}
	case tea.KeyCtrlO:
		return []byte{15}
	case tea.KeyCtrlP:
		return []byte{16}
	case tea.KeyCtrlR:
		return []byte{18}
	case tea.KeyCtrlS:
		return []byte{19}
	case tea.KeyCtrlT:
		return []byte{20}
	case tea.KeyCtrlU:
		return []byte{21}
	case tea.KeyCtrlV:
		return []byte{22}
	case tea.KeyCtrlW:
		return []byte{23}
	case tea.KeyCtrlX:
		return []byte{24}
	case tea.KeyCtrlY:
		return []byte{25}
	case tea.KeyCtrlZ:
		return []byte{26}
	default:
		// For unknown keys, try the string representation
		if s := msg.String(); len(s) == 1 {
			return []byte(s)
		}
		return nil
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

	case tea.KeyEscape:
		m.inputMode = false
		m.inputBuffer = ""
		return m, nil

	case tea.KeyBackspace:
		if len(m.inputBuffer) > 0 {
			m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
		}
		return m, nil

	case tea.KeyRunes:
		m.inputBuffer += string(msg.Runes)
		return m, nil

	case tea.KeySpace:
		m.inputBuffer += " "
		return m, nil
	}

	return m, nil
}

func (m *Model) createNewAgent(task string) {
	agent, err := m.wtManager.CreateWorktree(task)
	if err != nil {
		// TODO: Show error to user
		return
	}

	_, err = m.ptyManager.Spawn(agent.ID, agent.Worktree, agent.Task)
	if err != nil {
		// Clean up worktree on failure
		m.wtManager.RemoveWorktree(agent.ID)
		return
	}

	// Resize the new session
	termHeight := m.height - 2
	if termHeight < 1 {
		termHeight = 1
	}
	if session, ok := m.ptyManager.Get(agent.ID); ok {
		session.Resize(termHeight, m.width)
	}

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

	if !m.ready {
		return "Initializing..."
	}

	var b strings.Builder

	// Tab bar
	b.WriteString(m.renderTabBar())
	b.WriteString("\n")

	// Session output (takes remaining space)
	b.WriteString(m.renderSession())

	// Status bar / Input
	b.WriteString(m.renderStatusBar())

	return b.String()
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

	// Fill remaining space with background
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	return lipgloss.NewStyle().
		Width(m.width).
		Background(lipgloss.Color("236")).
		Render(tabBar)
}

func (m Model) renderSession() string {
	termHeight := m.height - 2 // Tab bar + status bar
	if termHeight < 1 {
		termHeight = 1
	}

	tab := m.currentTab()
	if tab == nil {
		return strings.Repeat("\n", termHeight)
	}

	session, ok := m.ptyManager.Get(tab.SessionID)
	if !ok {
		return strings.Repeat("\n", termHeight)
	}

	// Get the virtual terminal output
	output := session.Output()

	// Split into lines and take last termHeight lines
	lines := strings.Split(output, "\n")

	// Ensure we have exactly termHeight lines
	if len(lines) > termHeight {
		lines = lines[len(lines)-termHeight:]
	}
	for len(lines) < termHeight {
		lines = append(lines, "")
	}

	// Truncate lines that are too long
	for i, line := range lines {
		if len(line) > m.width {
			lines[i] = line[:m.width]
		}
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
	help := " ⌥← Prev │ ⌥→ Next │ ⌥N New │ ⌥M Merge │ ⌥W Close │ ⌥Q Quit"
	return statusStyle.Render(help)
}
