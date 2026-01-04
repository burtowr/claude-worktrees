---
description: Merge a CWT agent's work back to the base branch using AI-assisted merge
arguments:
  - name: agent_id
    description: The agent ID to merge (e.g., cwt-20250104-a1b2)
    required: true
---

# Merge CWT Agent

Merge an agent's worktree branch back to its base branch with intelligent conflict resolution.

## Prerequisites

You will receive an agent ID as `$ARGUMENTS`.

## Steps

### 1. Load Agent Information

```bash
REPO_ROOT=$(git rev-parse --show-toplevel)
AGENT_ID="$ARGUMENTS"

# Read state file
cat "$REPO_ROOT/.cwt/state.json"
```

Extract the agent's details:
- `branch`: The agent's git branch
- `baseBranch`: The branch to merge into
- `task`: What the agent was working on
- `worktree`: Path to the worktree

### 2. Validate Agent Exists

Verify the agent exists in the state and has commits:

```bash
# Check branch exists
git rev-parse --verify "$AGENT_BRANCH" 2>/dev/null

# Check for commits
git log --oneline "$BASE_BRANCH..$AGENT_BRANCH"
```

### 3. Delegate to Merge Orchestrator

Invoke the `merge-orchestrator` agent with the context:

```
I need to merge agent $AGENT_ID.

Agent Details:
- Branch: $BRANCH
- Base Branch: $BASE_BRANCH
- Task: $TASK
- Worktree: $WORKTREE

Please analyze the changes and perform the merge, resolving conflicts where possible.
```

The merge orchestrator will:
- Analyze the diff
- Check for conflicts
- Resolve trivial/complementary conflicts
- Escalate true conflicts
- Execute the merge

### 4. Update State

After successful merge, update the state file:
- Set agent status to "merged"
- Record merge timestamp
- Add to merge history

### 5. Report Result

Output the merge result:

```
✓ Successfully merged $AGENT_ID

Task: $TASK
Files merged: N
Conflicts resolved: N
Merge commit: $SHA

Use `/cwt:cleanup $AGENT_ID` to remove the worktree, or it will be cleaned up when you close the tab.
```

Or for failures:

```
✗ Merge requires manual intervention

See conflicts above. After resolving manually, run:
  git add .
  git commit

Then update the state file to mark the agent as merged.
```
