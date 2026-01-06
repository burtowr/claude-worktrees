# CWT Specification

Claude Worktree Tabs - A TUI for orchestrating multiple Claude Code agents in parallel using git worktrees.

## Features

### TUI Application

- **Tabbed Interface**: Textual-based TUI with tabs for managing multiple concurrent Claude sessions
- **Terminal Emulation**: Full PTY support using pyte for ANSI color/style rendering
- **Keyboard Navigation**:
  - `Ctrl+Q` - Quit application
  - `Ctrl+B` - Previous tab
  - `Ctrl+F` - Next tab
  - `Ctrl+N` - Create new worktree tab
- **Key Forwarding**: All keyboard input forwarded to active PTY session including special keys (arrows, home, end, page up/down, etc.)
- **30 FPS Refresh**: Display updates at 30 frames per second for smooth terminal rendering
- **Cursor Rendering**: Visual cursor position with reverse video highlight

### Dashboard Tab

The main tab displays a dashboard for managing worktrees instead of a direct Claude session.

**Layout**:
```
+----------------------------------------------------------+
|  GitStatusPanel - Branch: main | Uncommitted: 3 files    |
+----------------------------------------------------------+
|  WorktreeList (DataTable - arrow key navigation)         |
|  ID              Task           Status    +/-    Files   |
|  > cwt-20260106  Add auth       running   +50/-3   4     |
|    cwt-20260105  Fix tests      completed +20/-10  2     |
+----------------------------------------------------------+
|  ActionPanel - [m]erge [d]elete [v]iew diff [Enter]switch|
+----------------------------------------------------------+
|  MergeOutputPanel (shown during merge operations)        |
+----------------------------------------------------------+
|  KeyboardHelp - Quick reference for keybindings          |
+----------------------------------------------------------+
```

**Components**:
- **GitStatusPanel**: Current branch name, uncommitted changes count, and agent status summary (e.g., "2 running, 1 completed")
- **WorktreeList**: DataTable showing all agents with ID, task, status, and diff stats (+lines, -lines, files changed)
- **ActionPanel**: Shows available keyboard actions for selected worktree
- **MergeOutputPanel**: Inline display for merge orchestrator output (hidden until merge triggered)
- **KeyboardHelp**: Quick reference for all keybindings

**Dashboard Keybindings**:
| Key | Action | Description |
|-----|--------|-------------|
| `↑/↓` | Navigate | Select worktree in list |
| `m` | Merge | Run merge orchestrator, show output inline |
| `d` | Delete | Remove worktree and branch (with confirmation), closes associated tab |
| `v` | View Diff | Display diff output in panel |
| `Enter` | Switch | Jump to worktree's terminal tab |
| `r` | Refresh | Force reload state from disk |

**State Refresh**:
- Automatic refresh every 2 seconds from `.cwt/state.json`
- Diff stats cached and refreshed on file modification
- Manual refresh available with `r` key

**Merge Behavior**:
- Runs merge orchestrator as subprocess (not interactive PTY)
- Streams output to MergeOutputPanel inline
- Dashboard remains usable during merge

### Tab Auto-Naming

New worktree tabs are automatically named without prompting the user.

**Naming Scheme**:
| Scenario | Tab Name |
|----------|----------|
| First new tab | `new` |
| Second new tab | `new-1` |
| Third new tab | `new-2` |
| After first Claude request | Claude's auto-generated session name |

**Behavior**:
- New tabs created with `Ctrl+N` receive automatic names (`new`, `new-1`, etc.)
- When Claude processes the first user request, it generates a session name
- The tab automatically renames to Claude's session name
- When a `new-N` tab closes, remaining new tabs renumber to fill gaps

**Name Detection**:
- Monitors PTY output for ANSI title sequences (`ESC]0;titleBEL`)
- Detects Claude's session naming and triggers tab rename
- Only renames tabs that still have `new` or `new-N` names

### Git Worktree Management

- **Isolated Workspaces**: Each agent runs in its own git worktree with a dedicated branch
- **Agent ID Format**: `cwt-{YYYYMMDD}-{4-char-random-suffix}` (e.g., `cwt-20260106-a1b2`)
- **Branch Naming**: `cwt/{agent-id}/{slugified-task}` (task slug max 30 chars)
- **Worktree Location**: `.worktrees/{agent-id}/`
- **Lifecycle Operations**:
  - Create worktree with new branch from current HEAD
  - Remove worktree (normal then force)
  - Delete branch (normal then force)
  - Merge branch back to base with `--no-ff`

### Agent State Management

- **Persistent State**: JSON file at `.cwt/state.json`
- **State Version**: 1.0
- **Agent Statuses**:
  - `no_changes` - New worktree with no changes yet
  - `pending` - Created but not started
  - `running` - Agent actively working (has changes)
  - `completed` - Claude finished responding (auto-detected after 2s idle)
  - `merging` - Merge in progress
  - `merged` - Successfully merged
  - `failed` - Error occurred
- **Status Transitions**:
  - `no_changes` → `running`: When agent makes changes (commits or uncommitted)
  - `running` → `completed`: When Claude finishes responding (2 second idle timeout)
  - `completed` → `merging` → `merged`: During merge process
- **Tracked Metadata**:
  - Agent ID, branch name, worktree path
  - Task description
  - Base branch and base commit
  - Created timestamp
  - Merged timestamp (when applicable)
- **JSON Format**: camelCase keys for compatibility

### PTY Session Management

- **Claude Integration**: Spawns `claude` CLI with `--plugin-dir` flag
- **Terminal Settings**: `TERM=xterm-256color`, default 24x80 dimensions
- **Async Reading**: Background thread pool for non-blocking PTY reads
- **Color Conversion**: Pyte colors to Rich/Textual styles (hex, named, bright variants)
- **Style Support**: Bold, italic, underline, reverse video
- **Session Lifecycle**: Start reading, resize, write input, close/terminate

### Plugin Commands

- `/cwt:help` - Display available commands and keyboard shortcuts
- `/cwt:status` - Show formatted table of all agents with:
  - Agent ID, task, status
  - Diff stats (+lines, -lines, file count)
  - Summary counts by status
- `/cwt:merge <agent_id>` - Trigger AI-assisted merge for specified agent

### Merge Orchestrator (AI Agent)

- **Model**: Opus
- **Tools**: Read, Bash, Grep, Glob, Edit, Write
- **Conflict Analysis**:
  - Uses `git merge-tree` to detect conflicts
  - Categorizes conflicts into types:
    - **Type A (Trivial)**: Whitespace, import ordering, comments - auto-resolved
    - **Type B (Complementary)**: Different functions/sections added - combined
    - **Type C (True Conflicts)**: Same code modified differently - escalated to user
- **Merge Process**:
  1. Gather agent info from state file
  2. Analyze diff between base and agent branch
  3. Check for conflicts with merge-tree
  4. Categorize and resolve conflicts
  5. Execute merge with `--no-ff`
  6. Clean up worktree and branch
- **Quality Checks**:
  - No debug artifacts (console.log, print, debugger)
  - No duplicate/unused imports
  - No unintentional TODO markers
- **Escalation Protocol**: Clear conflict report with code snippets, analysis, and recommendations

## Runtime Requirements

- Python 3.10+
- Git repository (validates `.git` exists)
- `claude` CLI installed and in PATH
- Dependencies: textual, pyte, ptyprocess, rich

## File Structure

```
.cwt/
  state.json          # Agent state persistence
.worktrees/
  {agent-id}/         # Isolated worktree directories
plugin/
  .claude-plugin/
    plugin.json       # Plugin metadata
  agents/
    merge-orchestrator.md
  commands/
    help.md
    merge.md
    status.md
src/cwt/
  __init__.py
  __main__.py         # Entry point
  app.py              # Textual TUI
  pty_session.py      # PTY management
  worktree.py         # Git operations
```
