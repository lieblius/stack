# Build and test

```bash
go build -o st .
go vet ./...
```

# Code style

- Standard Go conventions, no special linting
- Cobra commands: one file per command (or related group like nav.go for up/down/top/bottom)
- Register all commands in `cmd/root.go` init()
- Use `internal/git.Run()` for git operations, never `os/exec` directly for git
- Use `internal/gh` for GitHub CLI operations
- Errors from read-only gh ops (PRForBranch): warn and continue. Write ops: hard fail.

# Architecture decisions

- Metadata lives in `git config --local` (`stack.branch.<name>.parent` and `.base`)
- `base` = fork-point SHA. Rebase triggers when `base != parent's current tip`
- Push before updating metadata (so crash leaves continue state for retry)
- Skip-worktree bits are saved/cleared before rebase, restored after (not on conflict)
- `st merge` runs `gh pr merge` interactively by default; `--ci` flag uses GitHub REST API for automation
- Stack nav comments in PR bodies use `<!-- st:stack -->` markers, spliced idempotently

# Testing

No automated test suite yet. Test manually on a repo with stacked branches.

# Common gotchas

- `gh pr merge` hangs in non-TTY contexts (Claude, CI). Always use `--ci` flag.
- `meta.AllTracked()` returns topological order (trunk to tips). Process in this order for rebases.
- `topoSort` returns an error on cycles. Callers must handle it.
- `PruneStale()` should be called at the start of sync, submit, rebase, merge.
