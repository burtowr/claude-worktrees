package pty

import (
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// Session represents a single PTY session running Claude Code
type Session struct {
	ID       string
	Workdir  string
	Task     string
	pty      *os.File
	cmd      *exec.Cmd
	buffer   *RingBuffer
	mu       sync.RWMutex
	done     chan struct{}
	onOutput func([]byte)
}

// RingBuffer is a fixed-size circular buffer for terminal output
type RingBuffer struct {
	data  []byte
	size  int
	start int
	len   int
	mu    sync.RWMutex
}

// NewRingBuffer creates a new ring buffer with the given size
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		data: make([]byte, size),
		size: size,
	}
}

// Write appends data to the ring buffer
func (r *RingBuffer) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, b := range p {
		pos := (r.start + r.len) % r.size
		r.data[pos] = b
		if r.len < r.size {
			r.len++
		} else {
			r.start = (r.start + 1) % r.size
		}
	}
	return len(p), nil
}

// Bytes returns the current buffer contents
func (r *RingBuffer) Bytes() []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]byte, r.len)
	for i := 0; i < r.len; i++ {
		result[i] = r.data[(r.start+i)%r.size]
	}
	return result
}

// String returns the buffer contents as a string
func (r *RingBuffer) String() string {
	return string(r.Bytes())
}

// NewSession creates a new PTY session
func NewSession(id, workdir, task string) *Session {
	return &Session{
		ID:      id,
		Workdir: workdir,
		Task:    task,
		buffer:  NewRingBuffer(1024 * 1024), // 1MB buffer
		done:    make(chan struct{}),
	}
}

// Start spawns the claude command in a PTY
func (s *Session) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cmd := exec.Command("claude")
	cmd.Dir = s.Workdir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}

	s.cmd = cmd
	s.pty = ptmx

	// Start reading from PTY
	go s.readLoop()

	return nil
}

// readLoop continuously reads from the PTY and writes to the buffer
func (s *Session) readLoop() {
	defer close(s.done)

	buf := make([]byte, 4096)
	for {
		n, err := s.pty.Read(buf)
		if err != nil {
			if err != io.EOF {
				// Log error but don't crash
			}
			return
		}
		if n > 0 {
			s.buffer.Write(buf[:n])
			if s.onOutput != nil {
				s.onOutput(buf[:n])
			}
		}
	}
}

// Write sends input to the PTY
func (s *Session) Write(data []byte) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.pty == nil {
		return 0, io.ErrClosedPipe
	}
	return s.pty.Write(data)
}

// Output returns the current buffer contents
func (s *Session) Output() string {
	return s.buffer.String()
}

// SetOutputCallback sets a callback for when new output arrives
func (s *Session) SetOutputCallback(cb func([]byte)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onOutput = cb
}

// Resize resizes the PTY
func (s *Session) Resize(rows, cols uint16) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.pty == nil {
		return nil
	}
	return pty.Setsize(s.pty, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

// Stop terminates the session
func (s *Session) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pty != nil {
		s.pty.Close()
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
