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
- Use `internal/provider` for hosting platform operations (GitHub, Forgejo)
- `internal/provider/pr.go` defines the `Host` interface and shared types
- `internal/provider/github.go` (gh CLI), `internal/provider/forgejo.go` (Go SDK)
- Provider is auto-detected from git remote URL in `cmd/provider.go`
- Errors from read-only provider ops (PRForBranch): warn and continue. Write ops: hard fail.

# Architecture decisions

- Metadata lives in `git config --local` (`stack.branch.<name>.parent` and `.base`)
- `base` = fork-point SHA. Rebase triggers when `base != parent's current tip`
- Push before updating metadata (so crash leaves continue state for retry)
- Skip-worktree bits are saved/cleared before rebase, restored after (not on conflict)
- `st merge` always merges via provider API; `--ci` flag skips confirmation prompts (same as `--yes`)
- Stack nav comments in PR bodies use `<!-- st:stack -->` markers, spliced idempotently

# Testing

No automated test suite yet. Verify changes with `go build` and `go vet` only.
Do NOT run `st` commands against any repo with a live remote. These commands merge PRs, force-push, and delete branches on real hosting platforms. If manual testing is absolutely needed, use a throwaway repo in /tmp with no remote.

# Common gotchas

- Forgejo may need a brief delay between sequential merges after rebase (handled with retry in provider).
- `meta.AllTracked()` returns topological order (trunk to tips). Process in this order for rebases.
- `topoSort` returns an error on cycles. Callers must handle it.
- `PruneStale()` should be called at the start of sync, submit, rebase, merge.
