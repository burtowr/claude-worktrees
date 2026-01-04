---
description: Show status of all CWT agents and their worktrees
---

# CWT Status

Display the current status of all Claude Worktree agents.

## Steps

1. Read the CWT state file:
```bash
REPO_ROOT=$(git rev-parse --show-toplevel)
cat "$REPO_ROOT/.cwt/state.json" 2>/dev/null || echo '{"agents":{}}'
```

2. For each agent in the state, gather additional info:
   - Check if worktree directory exists
   - Get recent commits on the branch
   - Check for completion marker `[CWT-DONE]` in commits
   - Get diff stats against base branch

3. Display a formatted table:

```
╭──────────────────────────────────────────────────────────────────────╮
│                         CWT Agent Status                              │
├──────────────────┬─────────────────────┬──────────┬───────────────────┤
│ Agent ID         │ Task                │ Status   │ Changes           │
├──────────────────┼─────────────────────┼──────────┼───────────────────┤
│ cwt-20250104-a1b2│ Add auth feature    │ running  │ +127 -12 (4 files)│
│ cwt-20250104-c3d4│ Write unit tests    │ completed│ +89 -0 (3 files)  │
╰──────────────────┴─────────────────────┴──────────┴───────────────────╯
```

4. Include summary:
   - Total agents: X
   - Running: X
   - Completed (ready to merge): X
   - Merged: X

## Notes

- Status is determined from both state file and git status
- An agent is "completed" if its most recent commit message starts with `[CWT-DONE]`
- Use `/cwt:merge <agent-id>` to merge a completed agent
