# st -- stacked pull requests

A CLI for managing stacked PRs with squash merge support. Works with GitHub and Forgejo. Create chains of dependent branches, submit them as PRs, and merge them one by one -- the tool handles rebasing, repointing PR bases, and cleanup automatically.

## Why

Squash merge rewrites commit history. When you merge a parent PR with squash, every child branch breaks because the fork point no longer exists. Existing tools handle this by detecting merged branches after the fact. `st` goes further -- it can merge PRs directly from the CLI and immediately rebase the rest of the stack.

## Install

```bash
curl -sSfL https://raw.githubusercontent.com/lieblius/stack/main/install.sh | sh
```

Or with Go:

```bash
go install github.com/liebl/stack@latest
```

Or build from source:

```bash
git clone https://github.com/lieblius/stack.git
cd stack
go build -o st .
```

### GitHub

Requires the `gh` CLI (https://cli.github.com). Auth is handled by `gh auth login`.

### Forgejo

Set the `FORGEJO_TOKEN` environment variable to a Forgejo API token with repo permissions. The provider is auto-detected from the git remote URL.

## Quick start

```bash
# Start a stack from main
git checkout main
st create feature-auth
# make changes, commit

st create feature-auth-tests
# make changes, commit

# Push everything and create PRs
st submit

# After code review, merge from the bottom up
st merge --all
```

## Commands

### Building the stack

| Command | Description |
|---|---|
| `st create <name> [-m msg] [-a]` | Create a branch stacked on the current one |
| `st track <branches...> [--force]` | Adopt existing branches into a stack (bottom to top) |

### Navigating

| Command | Description |
|---|---|
| `st list` | Show the stack tree with PR status |
| `st up [n]` | Move to child branch |
| `st down [n]` | Move to parent branch |
| `st top` | Jump to the tip of the stack |
| `st bottom` | Jump to the base of the stack |

### Submitting

| Command | Description |
|---|---|
| `st submit [--dry-run]` | Push all branches, create/update PRs, add stack navigation to PR bodies |

### Syncing

| Command | Description |
|---|---|
| `st sync [--dry-run]` | Pull trunk, detect merged PRs, rebase cascade, cleanup |
| `st rebase [--dry-run]` | Pull trunk, rebase stack (no merge detection) |

### Merging

| Command | Description |
|---|---|
| `st merge` | Merge the bottom PR, repoint children, rebase the rest, cleanup |
| `st merge --all` | Merge every PR in the stack, bottom to top |
| `st merge --strategy rebase` | Use a different merge strategy (default: squash) |
| `st merge --ci` | Skip confirmation prompts (same as `--yes`) |

### Cleanup

| Command | Description |
|---|---|
| `st delete [branch] [--remote]` | Delete a branch and reparent its children |
| `st untrack [branch]` | Remove from the stack without deleting the branch |
| `st continue` | Resume after resolving a rebase conflict |

## How it works

`st` stores branch relationships in `git config --local`:

```
stack.branch.feature-auth.parent = main
stack.branch.feature-auth.base = abc1234
stack.branch.feature-auth-tests.parent = feature-auth
stack.branch.feature-auth-tests.base = def5678
```

The `parent` field records which branch this one is stacked on. The `base` field records the parent's SHA at the time of last rebase -- this is how `st` knows which commits belong to each branch and when a rebase is needed.

When you run `st merge`:

1. Children of the target branch are repointed to trunk (so no PR is left targeting a deleted branch)
2. The PR is merged via the hosting provider's API
3. Trunk is pulled locally
4. The remaining stack is rebased onto the new trunk tip
5. All branches are force-pushed
6. The merged branch is cleaned up

## Reference implementations

These tools informed the design of `st`. The metadata model (parent + fork-point SHA in git config) and rebase detection logic (stored base != parent tip) are common across all of them.

- [Charcoal](https://github.com/screenplaydev/charcoal) -- Graphite OSS fork (TypeScript). Uses `parentBranchRevision` for the fork-point concept we call `base`. Most complete reference for submit, sync, and restack logic.
- [graphite-cli](https://github.com/screenplaydev/graphite-cli) -- Older Graphite CLI (TypeScript). Uses `prevRef` instead of `parentBranchRevision`. Informed the metadata storage approach.
- [st by clabby](https://github.com/clabby/st) -- Independent implementation (Rust). Simpler approach, stores metadata in TOML instead of git config.

None of these implement merge from the CLI -- they all expect merging through the GitHub UI and then running sync to detect and clean up. The `st merge` command (repoint children before merge, then rebase cascade) is specific to this tool.
