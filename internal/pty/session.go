package pty

import (
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

// Session represents a single PTY session running Claude Code
type Session struct {
	ID       string
	Workdir  string
	Task     string
	ptmx     *os.File
	cmd      *exec.Cmd
	term     vt10x.Terminal
	mu       sync.RWMutex
	done     chan struct{}
	rows     int
	cols     int
}

// NewSession creates a new PTY session
func NewSession(id, workdir, task string) *Session {
	return &Session{
		ID:      id,
		Workdir: workdir,
		Task:    task,
		done:    make(chan struct{}),
		rows:    24,
		cols:    80,
	}
}

// Start spawns the claude command in a PTY
func (s *Session) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create virtual terminal
	s.term = vt10x.New(vt10x.WithSize(s.cols, s.rows))

	cmd := exec.Command("claude")
	cmd.Dir = s.Workdir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(s.rows),
		Cols: uint16(s.cols),
	})
	if err != nil {
		return err
	}

	s.cmd = cmd
	s.ptmx = ptmx

	// Start reading from PTY into virtual terminal
	go s.readLoop()

	return nil
}

// readLoop continuously reads from the PTY and writes to the virtual terminal
func (s *Session) readLoop() {
	defer close(s.done)

	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if err != nil {
			if err != io.EOF {
				// Log error but don't crash
			}
			return
		}
		if n > 0 {
			s.mu.Lock()
			s.term.Write(buf[:n])
			s.mu.Unlock()
		}
	}
}

// Write sends input to the PTY
func (s *Session) Write(data []byte) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.ptmx == nil {
		return 0, io.ErrClosedPipe
	}
	return s.ptmx.Write(data)
}

// WriteString sends a string to the PTY
func (s *Session) WriteString(str string) (int, error) {
	return s.Write([]byte(str))
}

// Output returns the current terminal screen as a string
func (s *Session) Output() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.term == nil {
		return ""
	}

	return s.term.String()
}

// Resize resizes the PTY and virtual terminal
func (s *Session) Resize(rows, cols int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.rows = rows
	s.cols = cols

	if s.term != nil {
		s.term.Resize(cols, rows)
	}

	if s.ptmx != nil {
		return pty.Setsize(s.ptmx, &pty.Winsize{
			Rows: uint16(rows),
			Cols: uint16(cols),
		})
	}
	return nil
}

// Stop terminates the session
func (s *Session) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ptmx != nil {
		s.ptmx.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	return nil
}

// Done returns a channel that closes when the session ends
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// IsRunning returns true if the session is still running
func (s *Session) IsRunning() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// Rows returns current row count
func (s *Session) Rows() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rows
}

// Cols returns current column count
func (s *Session) Cols() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cols
}
