use crate::pty::Manager as PtyManager;
use crate::worktree::{Agent, Manager as WorktreeManager};
use anyhow::Result;
use crossterm::event::{self, Event, KeyCode, KeyEvent, KeyModifiers};
use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::Line,
    widgets::{Paragraph, Tabs},
    Frame,
};
use std::path::PathBuf;
use std::time::Duration;

/// Tab in the TUI
pub struct Tab {
    pub id: String,
    pub name: String,
    pub is_main: bool,
    pub agent: Option<Agent>,
}

/// Input mode for task entry
pub enum InputMode {
    Normal,
    Input { prompt: String, buffer: String },
}

/// Main application state
pub struct App {
    pub tabs: Vec<Tab>,
    pub active_tab: usize,
    pub pty_manager: PtyManager,
    pub wt_manager: WorktreeManager,
    pub input_mode: InputMode,
    pub should_quit: bool,
    pub term_rows: u16,
    pub term_cols: u16,
}

impl App {
    pub fn new(repo_root: PathBuf) -> Result<Self> {
        let wt_manager = WorktreeManager::new(repo_root.clone())?;
        let mut pty_manager = PtyManager::new();

        // Spawn main session
        pty_manager.spawn(
            "main".to_string(),
            repo_root.to_string_lossy().to_string(),
            "Main orchestrator".to_string(),
            24,
            80,
        )?;

        let mut tabs = vec![Tab {
            id: "main".to_string(),
            name: "Main".to_string(),
            is_main: true,
            agent: None,
        }];

        // Restore running agents
        for agent in wt_manager.list_agents() {
            if agent.status == crate::worktree::AgentStatus::Running {
                if pty_manager.spawn(
                    agent.id.clone(),
                    agent.worktree.to_string_lossy().to_string(),
                    agent.task.clone(),
                    24,
                    80,
                ).is_ok() {
                    tabs.push(Tab {
                        id: agent.id.clone(),
                        name: truncate(&agent.task, 15),
                        is_main: false,
                        agent: Some(agent.clone()),
                    });
                }
            }
        }

        Ok(Self {
            tabs,
            active_tab: 0,
            pty_manager,
            wt_manager,
            input_mode: InputMode::Normal,
            should_quit: false,
            term_rows: 24,
            term_cols: 80,
        })
    }

    /// Handle resize
    pub fn resize(&mut self, rows: u16, cols: u16) {
        self.term_rows = rows;
        self.term_cols = cols;
        // Account for tab bar and status bar
        let term_rows = rows.saturating_sub(2);
        self.pty_manager.resize_all(term_rows, cols);
    }

    /// Handle keyboard input
    pub fn handle_key(&mut self, key: KeyEvent) -> Result<()> {
        match &mut self.input_mode {
            InputMode::Input { buffer, .. } => {
                match key.code {
                    KeyCode::Enter => {
                        if !buffer.is_empty() {
                            let task = buffer.clone();
                            self.create_agent(&task)?;
                        }
                        self.input_mode = InputMode::Normal;
                    }
                    KeyCode::Esc => {
                        self.input_mode = InputMode::Normal;
                    }
                    KeyCode::Backspace => {
                        buffer.pop();
                    }
                    KeyCode::Char(c) => {
                        buffer.push(c);
                    }
                    _ => {}
                }
            }
            InputMode::Normal => {
                // Check for Ctrl modifiers
                if key.modifiers.contains(KeyModifiers::CONTROL) {
                    match key.code {
                        // Ctrl+B - previous tab
                        KeyCode::Char('b') => {
                            if self.active_tab > 0 {
                                self.active_tab -= 1;
                            }
                        }
                        // Ctrl+F - next tab
                        KeyCode::Char('f') => {
                            if self.active_tab < self.tabs.len() - 1 {
                                self.active_tab += 1;
                            }
                        }
                        // Ctrl+N - new agent
                        KeyCode::Char('n') => {
                            self.input_mode = InputMode::Input {
                                prompt: "Task: ".to_string(),
                                buffer: String::new(),
                            };
                        }
                        // Ctrl+W - close tab
                        KeyCode::Char('w') => {
                            if self.active_tab > 0 {
                                self.close_current_tab()?;
                            }
                        }
                        // Ctrl+G - merge
                        KeyCode::Char('g') => {
                            if self.active_tab > 0 {
                                self.merge_current_tab()?;
                            }
                        }
                        // Ctrl+Q or Ctrl+C - quit
                        KeyCode::Char('q') | KeyCode::Char('c') => {
                            self.should_quit = true;
                        }
                        _ => {
                            // Forward to PTY
                            self.forward_key(key)?;
                        }
                    }
                } else {
                    // Forward to active session
                    self.forward_key(key)?;
                }
            }
        }
        Ok(())
    }

    /// Forward key to active PTY session
    fn forward_key(&mut self, key: KeyEvent) -> Result<()> {
        let tab = &self.tabs[self.active_tab];
        if let Some(session) = self.pty_manager.get_mut(&tab.id) {
            let bytes = key_to_bytes(key);
            if !bytes.is_empty() {
                session.write(&bytes)?;
            }
        }
        Ok(())
    }

    /// Create a new agent
    fn create_agent(&mut self, task: &str) -> Result<()> {
        let agent = self.wt_manager.create_worktree(task)?;

        // Spawn PTY session
        let term_rows = self.term_rows.saturating_sub(2);
        self.pty_manager.spawn(
            agent.id.clone(),
            agent.worktree.to_string_lossy().to_string(),
            agent.task.clone(),
            term_rows,
            self.term_cols,
        )?;

        self.tabs.push(Tab {
            id: agent.id.clone(),
            name: truncate(&agent.task, 15),
            is_main: false,
            agent: Some(agent),
        });

        self.active_tab = self.tabs.len() - 1;
        Ok(())
    }

    /// Close current tab
    fn close_current_tab(&mut self) -> Result<()> {
        if self.active_tab == 0 {
            return Ok(()); // Don't close main
        }

        let tab = &self.tabs[self.active_tab];
        let id = tab.id.clone();

        // Remove PTY session
        self.pty_manager.remove(&id);

        // Remove worktree if agent
        if tab.agent.is_some() {
            let _ = self.wt_manager.remove_worktree(&id);
        }

        // Remove tab
        self.tabs.remove(self.active_tab);
        if self.active_tab >= self.tabs.len() {
            self.active_tab = self.tabs.len() - 1;
        }

        Ok(())
    }

    /// Merge current tab
    fn merge_current_tab(&mut self) -> Result<()> {
        if self.active_tab == 0 {
            return Ok(());
        }

        let tab = &self.tabs[self.active_tab];
        if let Some(agent) = &tab.agent {
            self.wt_manager.merge(&agent.id)?;
        }

        self.close_current_tab()
    }

    /// Render the UI
    pub fn render(&self, frame: &mut Frame) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(1), // Tab bar
                Constraint::Min(1),    // Terminal
                Constraint::Length(1), // Status bar
            ])
            .split(frame.size());

        self.render_tabs(frame, chunks[0]);
        self.render_terminal(frame, chunks[1]);
        self.render_status_bar(frame, chunks[2]);
    }

    fn render_tabs(&self, frame: &mut Frame, area: Rect) {
        let titles: Vec<Line> = self
            .tabs
            .iter()
            .map(|t| {
                let name = if t.is_main {
                    format!("● {}", t.name)
                } else {
                    t.name.clone()
                };
                Line::from(name)
            })
            .collect();

        let tabs = Tabs::new(titles)
            .select(self.active_tab)
            .style(Style::default().fg(Color::White).bg(Color::DarkGray))
            .highlight_style(
                Style::default()
                    .fg(Color::White)
                    .bg(Color::Blue)
                    .add_modifier(Modifier::BOLD),
            )
            .divider("|");

        frame.render_widget(tabs, area);
    }

    fn render_terminal(&self, frame: &mut Frame, area: Rect) {
        let tab = &self.tabs[self.active_tab];

        let content = if let Some(session) = self.pty_manager.get(&tab.id) {
            session.screen()
        } else {
            String::new()
        };

        // Split into lines and take what fits
        let lines: Vec<Line> = content
            .lines()
            .take(area.height as usize)
            .map(|l| Line::from(l.to_string()))
            .collect();

        let paragraph = Paragraph::new(lines);
        frame.render_widget(paragraph, area);
    }

    fn render_status_bar(&self, frame: &mut Frame, area: Rect) {
        let content = match &self.input_mode {
            InputMode::Input { prompt, buffer } => {
                format!("{}{}█", prompt, buffer)
            }
            InputMode::Normal => {
                " ^B Prev │ ^F Next │ ^N New │ ^G Merge │ ^W Close │ ^Q Quit".to_string()
            }
        };

        let paragraph = Paragraph::new(content)
            .style(Style::default().fg(Color::White).bg(Color::DarkGray));

        frame.render_widget(paragraph, area);
    }
}

/// Truncate string to max length
fn truncate(s: &str, max: usize) -> String {
    if s.len() <= max {
        s.to_string()
    } else {
        format!("{}…", &s[..max - 1])
    }
}

/// Convert key event to bytes for PTY
fn key_to_bytes(key: KeyEvent) -> Vec<u8> {
    match key.code {
        KeyCode::Char(c) => {
            if key.modifiers.contains(KeyModifiers::CONTROL) {
                // Ctrl+letter = letter - 'a' + 1
                if c.is_ascii_lowercase() {
                    vec![(c as u8) - b'a' + 1]
                } else {
                    vec![]
                }
            } else {
                c.to_string().into_bytes()
            }
        }
        KeyCode::Enter => vec![b'\r'],
        KeyCode::Backspace => vec![127],
        KeyCode::Tab => vec![b'\t'],
        KeyCode::Esc => vec![27],
        KeyCode::Up => vec![27, b'[', b'A'],
        KeyCode::Down => vec![27, b'[', b'B'],
        KeyCode::Right => vec![27, b'[', b'C'],
        KeyCode::Left => vec![27, b'[', b'D'],
        KeyCode::Home => vec![27, b'[', b'H'],
        KeyCode::End => vec![27, b'[', b'F'],
        KeyCode::PageUp => vec![27, b'[', b'5', b'~'],
        KeyCode::PageDown => vec![27, b'[', b'6', b'~'],
        KeyCode::Delete => vec![27, b'[', b'3', b'~'],
        _ => vec![],
    }
}

/// Poll for events with timeout
pub fn poll_event(timeout: Duration) -> Result<Option<Event>> {
    if event::poll(timeout)? {
        Ok(Some(event::read()?))
    } else {
        Ok(None)
    }
}
