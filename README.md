# panecom

**English** | [日本語](README.ja.md)

A CLI tool that abstracts pane-to-pane communication in a terminal multiplexer behind role names.

It lets AI agents (Claude, Codex, and friends) talk to each other without caring about pane IDs. Humans and AI sharing one terminal is a supported use case too.

## Supported multiplexers

| Multiplexer | Status |
|---|---|
| Zellij 0.45+ | Supported (uses `list-panes --json`, `new-pane --tab-id`) |
| tmux 3.2+ | Supported (uses `switch-client`, `capture-pane -p`) |

### Picking the multiplexer

Detection happens in this order:

| Priority | Method | Example |
|---|---|---|
| 1 | `--mux` flag | `panecom --mux tmux register developer` |
| 2 | `PANECOM_MUX` environment variable | `PANECOM_MUX=tmux panecom register developer` |
| 3 | Auto-detect: `ZELLIJ_SESSION_NAME` | Chosen automatically inside a Zellij session |
| 4 | Auto-detect: `TMUX` | Chosen automatically inside a tmux session |

If both environment variables are present, auto-detection prefers Zellij. Say which one you mean with `--mux` or `PANECOM_MUX`.

## Installation

### Build from source

```bash
git clone https://github.com/konono/panecom.git
cd panecom
go build -o bin/panecom .
```

Copy the resulting binary somewhere on your PATH.

### Release binaries

Once a release is published, grab the binary for your platform from the [Releases](https://github.com/konono/panecom/releases) page.

### go install

```bash
go install github.com/konono/panecom@latest
```

## Quick start

### Requirements

- Go 1.27+ (to build only)
- Zellij 0.45+ or tmux 3.2+
- Linux (amd64/arm64) or macOS (amd64/arm64)

### Minimal walkthrough

1. Start a multiplexer session:

```bash
zellij    # for Zellij
# or
tmux      # for tmux
```

2. In pane A, register as `developer`:

```bash
panecom register developer
```

3. Open another pane (pane B) and register it as `reviewer`. Start an agent such as Claude or Codex in pane B:

```bash
panecom register reviewer
codex  # or claude, etc.
```

To register pane B without moving to it, run `panecom register` from pane A. Within the current tab/window, use the arrow keys (or `h`/`j`/`k`/`l`) to pick the pane that sits up, down, left or right on screen, press Enter, then type the role name. While typing the role, `Tab` completes `developer` / `reviewer`: `rev`→`Tab` becomes `reviewer`, and `Tab` on an empty field cycles through the candidates. Any other role name works too. Selection stops at the edges instead of wrapping around. The selected pane is briefly tinted in Zellij and marked in tmux, and a preview of its screen is shown. The project is determined from the selected pane's working directory; if that can't be read, you are asked for the project path as well. `Esc` or `q` cancels pane selection, and `Esc` while typing the role cancels the registration. No command is sent to the target pane, but the registration is written to that project's `.panecom/` (created if missing) and the pane title becomes `panecom:<role>`. If you pick a pane belonging to another project, it is registered there, and the caller can reach it as `<project>/<role>` after `panecom link`.

Zellij colors return to normal once the selection screen closes. Colors you set individually with `set-pane-color` beforehand are not restored.

```text
$ panecom register
Select a pane  arrows/hjkl: move  Enter: select  Esc/q: cancel
Tab/window: app  (0)

  [app] terminal_1  claude  /path/to/app
> [app] terminal_2  codex   /path/to/app

Preview:
...
Role name (Tab: developer/reviewer): rev<Tab>
reviewer -> codex (/path/to/app)
```

> Register both panes **from the same session** and **the same project**. That is what puts them in the same namespace. The project is resolved by walking up from the working directory at registration time, so it doesn't matter which directory inside the repository you register from.

Check what you registered with `panecom whoami`. Moving around inside the same project still shows the namespace saved at registration time. If you move to a different project and the state store changes too, the registration isn't found and you get an error. If a script relies on the old role-only output, switch it to `panecom whoami --role`.

When you assign a role to an AI agent ("you're the reviewer", "go ahead as developer"), the agent follows the distributed panecom skill: it runs `panecom register reviewer` / `panecom register developer` itself and confirms the registration. The developer asks the reviewer for a review after implementing and before committing; the reviewer checks tests and actual behaviour, not just the diff. While work is in progress the reviewer checks the developer's screen roughly once an hour (this is not a timer that fires while the AI is idle). If you ask for collaboration with someone in another project, the agent can set up the necessary `link` itself as long as it knows the path.

```text
$ panecom whoami
role: reviewer
pane: terminal_2
namespace: /path/to/app
```

Panes registered by older versions may still have a subdirectory recorded as their namespace. `panecom list` detects such live legacy registrations in the same state store and points you at `panecom register <role>` run in that pane, or `panecom register` from another pane in the same tab. The same hint appears when role resolution fails. Detection alone does not modify any registration. Registrations left in a different state store, and ones with no `.git` or `.panecom` marker to prove they are the same project, are out of scope.

4. Communicate:

```bash
# from pane A
panecom resolve reviewer                  # → terminal_N (Zellij) / %N (tmux)
panecom tell reviewer "please review the code"  # goes to the agent in pane B
panecom dump reviewer                     # capture pane B's current screen
panecom focus reviewer                    # move to pane B
```

> **Careful**: `tell` / `send` inject a string plus Enter into the target pane. If the target is a bare shell, whatever you send runs as a command. Use `tell` when the input is meant for an agent.

### `tell` vs `send`

Use `tell` when the other side is an AI agent, and `send` when you want to drive the pane directly.

| | `tell` | `send` |
|---|---|---|
| Target | an AI agent that holds a role | any pane (shell, TUI, …) |
| Payload | prefixed with `[panecom:from=<your role>]` on line 1 | exactly the string you gave |
| Your own registration | required (error if unregistered) | not needed |

```bash
$ panecom tell reviewer "please take a look"
# the other pane receives:
#   [panecom:from=developer]
#   please take a look
```

When you send to someone in another project (`<alias>/<role>`), line 2 of the header carries your project path. Role names only mean something inside a namespace, so without it the recipient cannot work out where to reply.

```
[panecom:from=developer]
[panecom:project=/path/to/my project]
please take a look
```

The recipient runs `panecom link <alias> "/path/to/my project"` and replies with `panecom tell <alias>/developer`. Replying with the bare role name would hit the same-named role in their own project.

A header line holds one value: everything between `[panecom:<key>=` and the `]` at end of line. It is not split on whitespace, so paths containing spaces survive. If `tell` cannot determine your own project, it errors out instead of sending.

`send` adds no header because it is a general-purpose primitive — a header would corrupt input aimed at a shell or TUI. Conversely, using `send` for an agent leaves off the sender, so the recipient cannot tell where to reply.

### Delivery verification (both `tell` and `send`)

If the recipient isn't accepting input (detail view, pager, confirmation dialog…), the string you sent is silently dropped. `tell` / `send` capture the target screen before and after sending to verify delivery, and exit non-zero when they can't confirm it (on by default).

| Exit code | Meaning |
|---|---|
| 0 | the screen changed and the message is no longer in the input field (treated as accepted) |
| 1 | error: role not resolved, pane gone, … |
| 3 | the screen did not change at all (likely not accepted) |
| 4 | the message is still sitting in the input field (Enter didn't take) |

On a non-zero exit the last 20 lines of the target screen go to stderr. Use `--no-verify` to skip the check, or `--verify-timeout 10` for a slow-rendering recipient.

Note that exit 0 confirms "the screen moved", not that the recipient understood the request. Exit 4 is decided by checking whether the message is still at the bottom of the screen (line wrapping is handled), so UIs that draw hint lines below the input field may escape detection. The bias is deliberately towards missing a failure: misjudging an accepted message as failed would cause a duplicate send.

### Common errors

| Error | Cause | Fix |
|---|---|---|
| `not running inside a supported multiplexer` | run outside Zellij/tmux | run inside a multiplexer session |
| `unsupported multiplexer: X` | unknown `--mux` value | pass `zellij` or `tmux` |
| `ZELLIJ_PANE_ID not set` / `TMUX_PANE not set` | pane ID unavailable | run inside a multiplexer terminal pane |
| `role 'X' is not registered in namespace <path>` | target unregistered, or a different namespace | check that `<path>` in the message matches where the other side registered, and re-register from the same project |
| `role 'X' points to stale pane` | the target's pane was closed | have them run `panecom register X` again (the registration itself is removed at this point) |

## Commands

| Command | Description |
|---|---|
| `[--mux zellij\|tmux]` | choose the multiplexer explicitly (auto-detected when omitted) |
| `register` | pick a pane on screen and register it under a role name you type |
| `register <role>` | register the current pane under `<role>` |
| `whoami [--role]` | show the current pane's role, pane ID and the namespace recorded at registration (`--role` prints the role only) |
| `list [--all \| <project>] [--stale]` | list roles (yours / a link / everything). Stale roles are hidden by default; `--stale` shows them |
| `cleanup [--all] [-n]` | remove registrations whose pane is gone, plus state for sessions that ended (`--all` includes linked roles, `-n` is a dry run) |
| `link [-f] <project> <path>` | link another project (no arguments lists the links) |
| `unlink <project>` | remove a link |
| `tell [--no-verify] [--verify-timeout N] <role> <msg>` | send a message to an agent, prefixed with `[panecom:from=<your role>]` (delivery verified) |
| `resolve <role>` | resolve role → pane ID |
| `focus <role>` | move focus to the target pane |
| `open [-d right\|down] <role>` | open a new pane and register a role for it |
| `dump [--full] <role>` | capture the target pane's screen |
| `send [--no-verify] [--verify-timeout N] <role> <msg>` | type a literal string into the target pane (delivery verified) |
| `share [--full] <role>` | share your own screen with someone |
| `profile [-f] <name>` | build an environment from a profile |
| `skill install` | pick agents and scopes, then install the skill |
| `skill list` | list the skill installs panecom has recorded |
| `skill uninstall` | pick recorded installs and remove them |
| `skill sync [--check] <file>...` | regenerate the panecom skill block in the given files (only files that carry the markers; `--check` writes nothing and just reports whether they differ) |
| `update` | self-update to the latest release |
| `--version` | print the version |

## Option details

### dump / share

- `--full` — capture the whole scrollback, not just the visible screen

### open

- `-d right|down` — which way to split (default: unset, letting Zellij place the pane)

### send / tell

```bash
# add to ~/.zshrc (recommended)
setopt HIST_IGNORE_SPACE

# for bash
export HISTCONTROL=ignorespace
```

### profile

- `-f` / `--focus` — move focus to the tab that was created (default: return to the original tab)

### update

- Downloads the latest release from GitHub Releases and replaces the running binary in place
- Does nothing if the current version is already the latest

```bash
panecom update           # update to the latest version
panecom --version        # check the current version
```

### Role name constraints

A role name must match `[A-Za-z0-9][A-Za-z0-9._-]*`. `/` and `..` are rejected to prevent path traversal.

## Profiles

Define profiles in `.panecom.yaml` at the project root to bring up a development environment in one command. Only this file needs copying into a new repository — `.panecom/`, which holds session state, does not.

```yaml
profiles:
  review:
    panes:
      - role: developer
        cmd: claude
        foreground: true
      - role: reviewer
        cmd: codex
      - role: terminal
        direction: down
```

```bash
panecom profile review        # bring up the review environment
panecom profile -f review     # and move focus to the pane that asked for it
```

### Example use cases

**AI pair programming**: Claude and Codex reviewing each other

```yaml
profiles:
  pair:
    panes:
      - role: developer
        cmd: claude
        foreground: true
      - role: reviewer
        cmd: codex
```

**Development with a shared terminal**: an AI agent plus a pane for running commands

```yaml
profiles:
  dev:
    panes:
      - role: developer
        cmd: claude
        foreground: true
      - role: terminal
        direction: down
```

After `panecom profile dev`, Claude in the developer pane can run `panecom send terminal "make test"` to push a command into the terminal and `panecom dump terminal` to read the result.

### Profile fields

| Field | Description | Default |
|---|---|---|
| `role` | role name assigned to the pane (required) | — |
| `cmd` | command to run in the pane | none (a bare shell) |
| `direction` | split direction (`right` / `down`) | `right` |
| `foreground` | focus this pane when `-f` is given | `false` |

A profile may also carry `external_projects` (see "Working with another project" below).

### Config file precedence

| Priority | Path |
|---|---|
| High | `<project root>/.panecom.yaml` |
| Fallback | the legacy `.panecom/config.yaml` or `.panecom/config.yml` (only when the newer file is absent) |
| Low | `~/.config/panecom/config.yaml` or `config.yml` |

If `XDG_CONFIG_HOME` is set, the global config is looked up under `$XDG_CONFIG_HOME/panecom/`.

A profile of the same name defined in the project wins — the whole profile is replaced, not merged field by field. An existing `.panecom/config.yaml` is still read. To migrate, move just the config file to `.panecom.yaml` at the project root.

## Design

panecom is not a messaging server. There is no daemon.

```
developer ─────→ pane_2
reviewer  ─────→ pane_5
                    │
                    ▼
              Zellij / tmux
```

It is a thin wrapper that abstracts multiplexer pane IDs into stable role names.

### Namespace

A role's namespace is the session plus the **project root**. Roles in different projects never interfere.

The project root (the anchor) is found by walking up from the working directory at registration time:

1. the first `.git` found (a directory for a normal clone, a file for a worktree / submodule)
2. failing that, the first `.panecom/` found
3. failing that, the working directory itself

Because `.git` wins at every level, a leftover `.panecom/` in a subdirectory cannot split one repository in two. Also, **the home directory and the filesystem root are never treated as a project root** — a catch-all `~/.panecom` is a shared state location rather than a project, and anchoring there would merge unrelated projects into a single namespace.

Thanks to this, moving the working directory around inside the repository after registering does not change the namespace.

> **Changed in v0.3.0**: previously the working directory itself determined the namespace. After upgrading, namespace hashes change. Already-registered panes keep resolving because `panes/<id>` records the namespace directly, but **to make panes registered before the upgrade and newly registered panes see each other, re-run `panecom register` on the older side**.

### Working with another project

Link another project and you can address its roles as `<project>/<role>`. Since role names cannot contain `/`, everything before the last `/` is the project and everything after it is the role. An unqualified `<role>` still means your own namespace.

```bash
$ panecom link mcp ../rh_sc_mcp
mcp -> /path/to/rh_sc_mcp
  developer    terminal_12
  reviewer     terminal_13
  terminal     terminal_14

$ panecom tell mcp/reviewer "could you check the schema?"   # tell for agents
$ panecom send mcp/terminal "git status"                    # send for shell panes
```

`link` verifies the other side's roles on the spot and lists them. It fails if they haven't registered yet, so you don't find out only when you try to send. Use `-f` when you want the link in place before they start.

```bash
$ panecom link mcp ../rh_sc_mcp
panecom: no roles registered in /path/to/rh_sc_mcp (session: ope)
         pass --force to link anyway
```

`list` shows you what exists.

```bash
$ panecom list            # your own namespace
$ panecom list mcp        # a link
$ panecom list --all      # yours plus every link
$ panecom link            # list links (without resolving roles)
$ panecom unlink mcp
```

### Stale roles

A role whose pane was closed goes stale. Restarting the multiplexer renumbers every pane ID at once, which can leave all the roles in a namespace stale together. `list` hides stale entries by default and just shows the count.

```bash
$ panecom list
/path/to/project
  d            terminal_516
  (4 stale entries hidden: panecom list --stale, panecom cleanup)

$ panecom list --stale    # look at them
$ panecom cleanup -n      # see what would be removed (removes nothing)
$ panecom cleanup         # remove them
```

`tell` / `send` / `resolve` / `dump` / `share` / `focus` delete a registration the moment they hit a stale role. `cleanup` exists to sweep up whatever nobody touched.

A role whose liveness check itself failed is shown as `(unknown)` and is not deleted — that keeps an unresponsive multiplexer from wiping every role.

### State from ended sessions

State lives per session in `.panecom/sessions/<hash>/`. Restarting the multiplexer leaves the old session's directory behind. `list` only looks at the current session, so these accumulate out of sight.

Besides cleaning up roles, `cleanup` removes directories for sessions that are no longer running.

```bash
$ panecom cleanup -n
/path/to/project
  would remove developer    terminal_268
  would remove session lucky-drum
  would remove session quiet-zebra
4 stale entries, 2 dead sessions would be removed
```

- Zellij's EXITED sessions (resurrectable via attach) count as "not running" — resurrecting renumbers pane IDs anyway, so the old mapping is useless
- The current session is never removed, even if the multiplexer doesn't list it
- If listing sessions fails, session removal is skipped entirely
- Sessions belonging to a different multiplexer (state created under tmux, seen from Zellij) are out of scope because liveness cannot be checked
- Only your own project's store is touched. `--all` adds linked roles to the role sweep but never touches session state in a linked store

#### Properties

- **A link stores exactly one path.** Roles are read straight from the other side on every call, so there is nothing to redo when they re-register
- **It is one-way.** Nothing is written to the other side's state. For a round trip, they run `panecom link` too
- **It survives sessions.** `link` only records a path, so it stays valid across Zellij / tmux restarts
- The path you pass is **normalised to the project root**. Point it at a subdirectory like `../rh_sc_mcp/src` and the anchor rules above walk up for you
- The destination pane must be **in the same multiplexer session**, because pane IDs are only valid within a session

#### Recording a recurring link in config

When running `link` every time gets old, declare it in a profile. The links are created when `panecom profile <name>` starts.

```yaml
profiles:
  dev:
    external_projects:
      mcp: ../rh_sc_mcp
    panes:
      - role: developer
        cmd: 'claude --allow-dangerously-skip-permissions'
        foreground: true
      - role: reviewer
        cmd: 'codex --yolo'
```

Paths resolve relative to the project root holding the config. Use relative paths, not absolute ones, if you plan to commit and share it. The other side is normally not running yet at startup, so materialising here does not verify that the roles exist (equivalent to `link -f`).

The entries under `.panecom/externals/` are machine-specific absolute paths, so they belong in `.gitignore`. Links you want to share go in the config.

### State

The state directory is chosen in this order:

1. the `PANECOM_STATE_DIR` environment variable (wins if set)
2. an existing `.panecom/` found by walking up from the anchor above
3. otherwise a new `<anchor>/.panecom/`

Because the search starts at the anchor rather than the cwd, a leftover `.panecom/` in a subdirectory can never shadow the project's real state directory.

```
.panecom.yaml            # shareable profile config (project root)
.panecom/
├── externals/           # links to other projects (persist across sessions)
│   └── <anchor-hash>/
│       └── <alias>      # the linked project root
├── sessions/            # runtime state (generated)
│   └── <session-hash>/
│       ├── panes/
│       ├── namespaces/
│       └── .session
```

`sessions/` is runtime data. Add it to `.gitignore`:

```
.panecom/sessions/
# leftovers from exec, removed in v3.0.0. Nothing creates these any more,
# but existing data contains commands and their output
.panecom/exec/
.panecom/exec.lock
```

Versions before v3.0.0 also created `.panecom/exec/` and `.panecom/exec.lock`. Removing the `exec` command means nothing creates them any more, but **existing data stays on disk**. It holds the commands that were run along with their stdout / stderr, so keep them ignored and delete them if you don't need them.

## Safety

panecom performs **direct string injection and arbitrary command execution** against the target pane.

- **`send` / `tell`** inject a string and Enter into the target pane. If the target is a shell, what you send runs as a command. This is not a message API, and it operates outside the caller's sandbox and permission controls
- **`dump` / `share`** transfer the contents of the target pane's screen, which may include secrets that happen to be displayed

**Trust boundary**: sending to a registered role allows arbitrary action with the privileges of the target pane's user. Only send to roles you trust. For multi-agent collaboration this is the intended behaviour.

## Skill for AI agents

An installed panecom can distribute the skill itself. `panecom skill install` shows whether `~/.claude` and `~/.codex` exist and offers a checklist of Claude / Codex × user (all projects) / project (the current project). Move with the up/down arrows (or `j`/`k`), check with Space, then move to `Install selected` at the bottom and press Enter to apply. `Tab` jumps to the apply button, and `Esc` or `q` cancels. You can give each agent a different scope, or install the same agent into both user and project scope. The directory check is not a check that the agent itself is installed. It runs outside a multiplexer too, but it needs an interactive terminal.

```bash
panecom skill install
# [x] Claude / User (all projects)
# [ ] Claude / Project (current repository)
# [ ] Codex  / User (all projects)
# [x] Codex  / Project (current repository)
# > [ Install selected (2) ]
```

### After changing the skill

`skill install` is an interactive UI, so it cannot mechanically keep files already committed to a repository (such as this repository's `AGENTS.md`) up to date. Use `skill sync` for that.

```bash
panecom skill sync AGENTS.md           # regenerate the block
panecom skill sync --check AGENTS.md   # exit non-zero if out of date (for CI)
```

Anything outside the markers (`<!-- panecom-skill:start -->` … `<!-- panecom-skill:end -->`) is left alone.

**Only files that carry the markers are eligible.** A file without them is an error (use `skill install` to add one). Claude skill files in particular are a different format — the whole file is the skill rather than a block — so they are excluded: passing one to `sync` would append a block in the wrong format, and that is rejected explicitly. The embedding source `.claude/skills/panecom.md` is excluded for the same reason.

When you `sync` a file managed by `skill install`, the manifest record is updated too. A stale record would make `skill uninstall` / re-install refuse with "changed since installation".

In this repository the CI `skill-sync` job runs `--check`, so changing `.claude/skills/panecom.md` and forgetting to regenerate `AGENTS.md` fails the build.

Your chosen install targets are recorded in `~/.config/panecom/skill-installs.json`. The commands below inspect and remove them. `list` also shows installs made by other means (an older script, say) as `untracked`, as far as the current user/project scope reveals them. Choosing an existing install with identical content adopts it as managed, and uninstalling leaves a file that was already there in place. An existing file with local edits has its pre-install content and permissions saved and restored. Existing Claude files are replaced wholesale at install time, and the selection screen says so. A symlink target is replaced by a regular file, and uninstall does not restore the symlink. If the target is already deleted, only the tracking record is removed. Uninstall aborts if the skill body was edited afterwards, as a safeguard.

```bash
panecom skill list
panecom skill uninstall
```

Install uses the skill bundled into the running panecom binary. To get the newest one, update the binary first with `panecom update` or `go install .`, then re-run `panecom skill install`. Agents pick up the new instructions on their next session.

To regenerate this repository's `AGENTS.md`, run `scripts/deploy-skill.sh` from the source tree as before.

```bash
./scripts/deploy-skill.sh
```

The install targets are:

| Target | Path | How to update |
|---|---|---|
| Claude Code (user) | `~/.claude/skills/panecom.md` | `skill install` / script |
| Claude Code (project) | `<project>/.claude/skills/panecom.md` | `skill install` |
| Codex (project) | `<project>/AGENTS.md` | `skill install` / source-tree script |
| Codex (user) | `~/.codex/instructions.md` | `skill install` / script |

Re-running is idempotent (the marker-delimited section is overwritten).

## License

MIT License — see [LICENSE](LICENSE).
