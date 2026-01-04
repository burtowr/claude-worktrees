---
description: Show CWT plugin help and available commands
---

# Claude Worktree Agents (CWT) Help

CWT enables running multiple Claude Code agents in parallel, each in its own git worktree.

## How It Works

1. **Main Tab**: The first tab runs in your repo root. Use it for orchestration and merging.
2. **Agent Tabs**: Additional tabs run in isolated git worktrees, each on its own branch.
3. **Merge**: When an agent completes, merge its work back using the merge orchestrator.

## Keyboard Shortcuts (in CWT TUI)

| Shortcut | Action |
|----------|--------|
| `⌥←` | Previous tab |
| `⌥→` | Next tab |
| `⌥N` | New agent (create worktree) |
| `⌥M` | Merge current tab's agent |
| `⌥W` | Close current tab |
| `⌥Q` | Quit CWT |

## Plugin Commands (in Main tab)

| Command | Description |
|---------|-------------|
| `/cwt:status` | Show all agents and their status |
| `/cwt:merge <id>` | Merge an agent's work with AI assistance |
| `/cwt:help` | Show this help message |

## Agent Completion

Agents can signal completion by creating a commit with message starting with `[CWT-DONE]`:

```bash
git commit --allow-empty -m "[CWT-DONE] Completed: description of work"
```

## File Locations

- **State file**: `.cwt/state.json` - tracks all agents
- **Worktrees**: `.worktrees/` - contains agent worktrees
- **Plugin**: `plugin/` - Claude Code plugin for merge commands

## Workflow Example

1. Run `cwt` in your git repo
2. Press `⌥N` to create a new agent, enter task description
3. Work with the agent in its tab
4. When done, press `⌥M` or switch to Main and run `/cwt:merge <id>`
5. The merge orchestrator will handle conflicts intelligently
6. Press `⌥W` to close the merged tab (cleans up worktree)

## Tips

- Keep agent tasks focused and independent to minimize conflicts
- Use descriptive task names - they become part of the branch name
- The merge orchestrator understands code semantics, not just text diffs
- Check `/cwt:status` periodically to see agent progress
