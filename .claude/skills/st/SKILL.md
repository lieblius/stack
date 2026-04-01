---
name: st
description: Manage stacked pull requests using the st CLI. Use when working with stacked branches, creating PR stacks, submitting, merging, or navigating branch stacks.
argument-hint: "[command or description of what to do]"
---

# Stacked PR Management with `st`

The `st` CLI manages stacked pull requests -- chains of dependent branches where each PR targets the branch below it. The tool handles the critical problem of squash merges breaking child branch histories.

Before using, verify `st` is installed by running `st --help`. If not found, tell the user they need to install it.

All commands must be run from within a git repository that has a GitHub remote.

## Commands

### Creating and tracking
- `st create <name> [-m msg] [-a]` -- create a branch stacked on the current branch, optionally committing staged changes
- `st track <branches...> [--trunk main] [--force]` -- adopt pre-existing branches into a stack (bottom to top order). **Multiple branches in one call creates a linear chain** (trunk -> branch1 -> branch2). To track sibling branches (same parent), run separate `st track` calls.

### Viewing
- `st list` -- show the stack tree with PR status and commit counts

### Navigation
- `st up [n]` -- move to child branch (prompts if multiple children)
- `st down [n]` -- move to parent branch
- `st top` -- jump to the tip of the stack
- `st bottom` -- jump to the base of the stack

### Submitting
- `st submit [--dry-run]` -- push all tracked branches, create/update PRs, update stack navigation in PR bodies

### Syncing
- `st sync [--dry-run]` -- pull trunk, detect merged PRs, reparent orphans, rebase cascade, force push, cleanup
- `st rebase [--dry-run]` -- pull trunk, rebase stack onto new trunk tip (no merge detection)

### Merging
- `st merge [--ci] [--all] [--strategy squash|merge|rebase]` -- merge the bottom PR, repoint children, rebase cascade, cleanup
- `st merge --ci` -- non-interactive merge via GitHub API (required when running from Claude)
- `st merge --ci --all` -- merge entire stack non-interactively

### Cleanup
- `st delete [branch] [--remote]` -- delete branch and reparent children
- `st untrack [branch]` -- remove from stack metadata, keep the git branch

### Recovery
- `st continue` -- resume after resolving a rebase conflict, then run `st sync`

## Important: Non-interactive merging

When running `st merge` from Claude Code, always use the `--ci` flag. Without it, `gh pr merge` runs interactively and requires TTY input that Claude cannot provide. The `--ci` flag uses the GitHub REST API directly.

```bash
st merge --ci
st merge --ci --all
```

## Critical: never use manual git rebase

Never run `git rebase` directly on branches managed by `st`. The tool tracks fork-point SHAs in metadata, and manual rebases don't update them. This causes stale metadata, which leads to duplicate commits on the next `st sync` or `st rebase`.

Instead, always use `st rebase` or `st sync`. These handle all common cases:
- Trunk updated: `st rebase` or `st sync` pulls trunk and cascades rebases through the stack
- Amended a commit on a lower branch: `st rebase` detects the tip changed and cascades the rebase up through all children
- Someone merged a PR on GitHub: `st sync` detects the merge, reparents orphans, and rebases

If you accidentally ran a manual `git rebase`, run `st sync` immediately to let it attempt to reconcile metadata.

### Safe commit surgery on a managed branch

When you need to drop, reorder, or amend commits in ways `st` doesn't support natively:

1. Only operate on the single branch that needs changes. Never touch unrelated branches.
2. `st untrack <branch>` -- remove just that branch from st metadata
3. Perform git surgery (interactive rebase, `git rebase --onto`, etc.)
4. `st track <branch> --trunk <parent> --force` -- re-track with fresh metadata
5. `st rebase` -- cascade changes to any children if needed

## Typical workflows

### Start a new stack
```bash
git checkout main
st create feature-part-1
# make changes, commit
st create feature-part-2
# make changes, commit
st submit
```

### After code review, merge the stack
```bash
st merge --ci --all
```

### After someone merges a PR on GitHub
```bash
st sync
```

### Add a branch to an existing stack
```bash
st top
st create next-part
# make changes, commit
st submit
```

## Metadata

Stack relationships are stored in `git config --local` as `stack.branch.<name>.parent` and `stack.branch.<name>.base`. The `base` is the fork-point SHA used to determine which commits belong to each branch during rebase.
