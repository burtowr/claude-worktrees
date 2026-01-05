use anyhow::{Context, Result};
use portable_pty::{native_pty_system, CommandBuilder, PtyPair, PtySize};
use std::collections::HashMap;
use std::io::{Read, Write};
use std::sync::{Arc, Mutex};
use std::thread;
use vt100::Parser;

/// A single PTY session running Claude
pub struct Session {
    pub id: String,
    pub workdir: String,
    pub task: String,
    pty_pair: PtyPair,
    parser: Arc<Mutex<Parser>>,
    writer: Box<dyn Write + Send>,
    rows: u16,
    cols: u16,
}

impl Session {
    /// Create and start a new PTY session
    pub fn new(id: String, workdir: String, task: String, rows: u16, cols: u16) -> Result<Self> {
        let pty_system = native_pty_system();

        let pty_pair = pty_system
            .openpty(PtySize {
                rows,
                cols,
                pixel_width: 0,
                pixel_height: 0,
            })
            .context("Failed to open PTY")?;

        let mut cmd = CommandBuilder::new("claude");
        cmd.cwd(&workdir);
        cmd.env("TERM", "xterm-256color");

        let _child = pty_pair
            .slave
            .spawn_command(cmd)
            .context("Failed to spawn claude")?;

        let reader = pty_pair
            .master
            .try_clone_reader()
            .context("Failed to clone PTY reader")?;

        let writer = pty_pair
            .master
            .take_writer()
            .context("Failed to take PTY writer")?;

        let parser = Arc::new(Mutex::new(Parser::new(rows, cols, 1000)));

        // Spawn reader thread
        let parser_clone = Arc::clone(&parser);
        thread::spawn(move || {
            read_pty(reader, parser_clone);
        });

        Ok(Self {
            id,
            workdir,
            task,
            pty_pair,
            parser,
            writer,
            rows,
            cols,
        })
    }

    /// Write data to the PTY
    pub fn write(&mut self, data: &[u8]) -> Result<()> {
        self.writer.write_all(data)?;
        self.writer.flush()?;
        Ok(())
    }

    /// Get the current terminal screen content
    pub fn screen(&self) -> String {
        let parser = self.parser.lock().unwrap();
        let screen = parser.screen();
        screen.contents()
    }

    /// Get screen with styles for rendering
    pub fn screen_rows(&self) -> Vec<String> {
        let parser = self.parser.lock().unwrap();
        let screen = parser.screen();

        let mut rows = Vec::new();
        for row in 0..screen.size().0 {
            let mut line = String::new();
            for col in 0..screen.size().1 {
                let cell = screen.cell(row, col).unwrap();
                line.push(cell.contents().chars().next().unwrap_or(' '));
            }
            // Trim trailing spaces
            let trimmed = line.trim_end();
            rows.push(trimmed.to_string());
        }
        rows
    }

    /// Resize the PTY
    pub fn resize(&mut self, rows: u16, cols: u16) -> Result<()> {
        self.rows = rows;
        self.cols = cols;

        self.pty_pair
            .master
            .resize(PtySize {
                rows,
                cols,
                pixel_width: 0,
                pixel_height: 0,
            })
            .context("Failed to resize PTY")?;

        self.parser.lock().unwrap().set_size(rows, cols);
        Ok(())
    }

    pub fn rows(&self) -> u16 {
        self.rows
    }

    pub fn cols(&self) -> u16 {
        self.cols
    }
}

/// Read from PTY and feed into parser
fn read_pty(mut reader: Box<dyn Read + Send>, parser: Arc<Mutex<Parser>>) {
    let mut buf = [0u8; 4096];
    loop {
        match reader.read(&mut buf) {
            Ok(0) => break, // EOF
            Ok(n) => {
                let mut parser = parser.lock().unwrap();
                parser.process(&buf[..n]);
            }
            Err(_) => break,
        }
    }
}

/// Manager for multiple PTY sessions
pub struct Manager {
    sessions: HashMap<String, Session>,
}

impl Manager {
    pub fn new() -> Self {
        Self {
            sessions: HashMap::new(),
        }
    }

    /// Spawn a new session
    pub fn spawn(&mut self, id: String, workdir: String, task: String, rows: u16, cols: u16) -> Result<()> {
        let session = Session::new(id.clone(), workdir, task, rows, cols)?;
        self.sessions.insert(id, session);
        Ok(())
    }

    /// Get a session by ID
    pub fn get(&self, id: &str) -> Option<&Session> {
        self.sessions.get(id)
    }

    /// Get a mutable session by ID
    pub fn get_mut(&mut self, id: &str) -> Option<&mut Session> {
        self.sessions.get_mut(id)
    }

    /// Remove a session
    pub fn remove(&mut self, id: &str) -> Option<Session> {
        self.sessions.remove(id)
    }

    /// Resize all sessions
    pub fn resize_all(&mut self, rows: u16, cols: u16) {
        for session in self.sessions.values_mut() {
            let _ = session.resize(rows, cols);
        }
    }

    /// List all session IDs
    pub fn list(&self) -> Vec<&String> {
        self.sessions.keys().collect()
    }
}

impl Default for Manager {
    fn default() -> Self {
        Self::new()
    }
}
