mod app;
mod pty;
mod worktree;

use anyhow::{Context, Result};
use app::App;
use crossterm::{
    event::{Event, KeyEventKind},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use ratatui::prelude::*;
use std::io::stdout;
use std::path::PathBuf;
use std::process::Command;
use std::time::Duration;

fn main() -> Result<()> {
    // Find git root
    let repo_root = find_git_root()?;

    // Check claude is available
    check_claude_installed()?;

    // Initialize terminal
    enable_raw_mode()?;
    let mut stdout = stdout();
    execute!(stdout, EnterAlternateScreen)?;

    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend)?;

    // Create app
    let mut app = App::new(repo_root)?;

    // Get initial size
    let size = terminal.size()?;
    app.resize(size.height, size.width);

    // Main loop
    let result = run_app(&mut terminal, &mut app);

    // Cleanup
    disable_raw_mode()?;
    execute!(terminal.backend_mut(), LeaveAlternateScreen)?;

    result
}

fn run_app<B: Backend>(terminal: &mut Terminal<B>, app: &mut App) -> Result<()> {
    loop {
        // Draw
        terminal.draw(|frame| {
            app.render(frame);
        })?;

        // Handle events
        if let Some(event) = app::poll_event(Duration::from_millis(50))? {
            match event {
                Event::Key(key) if key.kind == KeyEventKind::Press => {
                    app.handle_key(key)?;
                }
                Event::Resize(width, height) => {
                    app.resize(height, width);
                }
                _ => {}
            }
        }

        if app.should_quit {
            break;
        }
    }

    Ok(())
}

fn find_git_root() -> Result<PathBuf> {
    let output = Command::new("git")
        .args(["rev-parse", "--show-toplevel"])
        .output()
        .context("Failed to run git")?;

    if !output.status.success() {
        // Try current directory
        let cwd = std::env::current_dir()?;
        if cwd.join(".git").exists() {
            return Ok(cwd);
        }
        anyhow::bail!("Not in a git repository");
    }

    let path = String::from_utf8_lossy(&output.stdout).trim().to_string();
    Ok(PathBuf::from(path))
}

fn check_claude_installed() -> Result<()> {
    if Command::new("which")
        .arg("claude")
        .output()
        .map(|o| o.status.success())
        .unwrap_or(false)
    {
        Ok(())
    } else {
        anyhow::bail!("'claude' command not found. Please install Claude Code first.")
    }
}
