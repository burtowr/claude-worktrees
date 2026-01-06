# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

CWT (Claude Worktree Tabs) is a Python TUI for orchestrating multiple Claude Code agents in parallel, each in isolated git worktrees. It provides a tabbed interface for managing concurrent agent sessions with intelligent merge capabilities.

## Commands

```bash
# Install in development mode
pip install -e .

# Run the TUI (must be in a git repo with claude CLI available)
cwt

# Run claude with the plugin
claude --plugin-dir plugin
```

## Architecture

**Core modules** (`src/cwt/`):
- `app.py` - Textual TUI app with `DashboardWidget`, `TerminalWidget`, and `CWTApp` for tab management
- `pty_session.py` - PTY session management using pyte for terminal emulation, converts ANSI to Rich styles
- `worktree.py` - Git worktree operations, agent state persistence (`.cwt/state.json`), branch lifecycle, diff stats

**Plugin** (`plugin/`):
- `agents/merge-orchestrator.md` - AI agent for intelligent conflict detection and merge resolution
- `commands/` - Slash commands: `/cwt:help`, `/cwt:status`, `/cwt:merge`

**Agent workflow**:
1. Each agent gets a unique ID (`cwt-{YYYYMMDD}-{suffix}`) and branch (`cwt/{id}/{task-slug}`)
2. Agents run in isolated worktrees under `.worktrees/`
3. Merge orchestrator analyzes diffs and resolves conflicts (trivial/complementary auto-resolved, true conflicts escalated)

**State**: Agent metadata stored in `.cwt/state.json` with camelCase keys for JSON compatibility.

## Key Bindings

**Global:**
- `Ctrl+Q` - Quit
- `Ctrl+B` - Previous tab
- `Ctrl+F` - Next tab
- `Ctrl+N` - Create new worktree tab

**Dashboard (main tab):**
- `↑/↓` - Navigate worktree list
- `m` - Merge selected worktree
- `d` - Delete selected worktree
- `v` - View diff for selected worktree
- `Enter` - Switch to worktree's terminal tab
- `r` - Refresh dashboard
