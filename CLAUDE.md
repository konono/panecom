# panecom development guide

**English** | [日本語](CLAUDE.ja.md)

## Build and test

```bash
go build -o bin/panecom .
go install .
go vet ./...
go test ./... -race
gofmt -w main.go main_test.go
```

## The panecom state directory

Layout under `.panecom/`:

- `sessions/` — the role ↔ pane mapping (**do not delete**)
- Profile configuration lives in `.panecom.yaml` at the project root (the legacy `.panecom/config.yaml` is still read)

v3.0.0 removed the `exec` command, so `exec/` and `exec.lock/` are no longer created. Delete them if an older environment still has them.

### Before you delete state

Deleting `.panecom/sessions/` **loses every role registration**. Don't. To tidy up stale registrations, use `panecom cleanup`.

## Changing the skill

After editing `.claude/skills/panecom.md`, regenerate `AGENTS.md`:

```bash
go run . skill sync AGENTS.md
```

`AGENTS.md` is an installed copy of the skill and does not follow the source on its own. CI's `skill-sync` job runs `skill sync --check AGENTS.md`, so forgetting this fails the build.

The skill is written in English, but it must keep working the same way for Japanese users: trigger phrases are listed in both languages, and the skill tells the agent to reply in the language the user is speaking. Preserve both properties when you edit it.

## Documentation

English is the source of truth; the Japanese files are mirrors.

| English | Japanese |
|---|---|
| `README.md` | `README.ja.md` |
| `CLAUDE.md` | `CLAUDE.ja.md` |

When you change one side, update the other in the same PR. Nothing checks this automatically.

## Branch policy

- Never push directly to main
- Work on a branch and merge through a PR
- release-please opens the release PR automatically

## Mixed macOS / Linux environments

When a container (Linux) and macOS share the same Zellij session:
- Install the binary onto PATH with `go install .` (on both OSes)
- The `panecom` command assumes it is on PATH
