<!-- panecom-skill:start -->

# panecom

A pane-to-pane communication tool for terminal multiplexers (Zellij / tmux). You address the other side by role name, never by pane ID.

## Language

**Reply in the language the user is using.** This skill is written in English, but that says nothing about the language you speak. If the user writes in Japanese, answer in Japanese and send Japanese in `panecom tell` messages; the same goes for any other language. Role names, command names and the `[panecom:...]` headers are always ASCII and never translated.

## Important: when to use this

**Do not use panecom on your own initiative.** Use it only when the user explicitly asks, for example:

| Intent | English | Japanese |
|---|---|---|
| Open a shared terminal | "make a shared terminal", "open a terminal", "create a new pane" | 「共有ターミナルを作って」「terminal 開いて」「新しい pane を作って」 |
| Work together in the terminal | "let's work in the terminal together" | 「一緒にターミナルで作業しよう」 |
| Name the tool | "do X with panecom" | 「panecom で〜して」 |
| Address another role | "send it to the reviewer", "share it with the developer" | 「reviewer に送って」「developer に共有して」 |
| Assign you a role | "you're the reviewer", "go ahead as developer" | 「あなたがレビュワーね」「developer として進めて」 |
| Collaborate across projects | "talk it over with the reviewer in the other project" | 「別プロジェクトの reviewer と相談して」 |

**Run ordinary commands in your own environment (the Bash tool and friends).** Going through panecom puts them outside your sandbox and permission controls.

## Taking part as developer / reviewer

When the user names you the reviewer ("reviewer", 「レビュワー」), run `panecom register reviewer` in your own pane. For developer ("developer", 「開発者」), run `panecom register developer`. On success, confirm the role and namespace with `panecom whoami`. On failure, tell the user why and do not treat yourself as registered. When the user wants to pick a pane themselves, that is `panecom register` with no arguments.

When `developer` and `reviewer` collaborate inside the same project, work like this by default:

- All agent-to-agent traffic goes through `panecom tell` (not `send` — see "tell vs send" below).
- The reviewer agrees on an approach with the developer and sends implementation requests via `panecom tell developer`. When the developer is unsure about a decision, they consult via `panecom tell reviewer`.
- After implementing and before committing, the developer asks for a review. The reviewer checks tests and real behaviour, not just the diff, and **returns the result with `panecom tell developer`**. Writing it in your own pane and stopping there doesn't count (see "Requests and replies").
- Based on the review, the reviewer gives the go-ahead to commit, push and open a PR, and the developer carries it out. Honour any confirmation or approval conditions set by the user or by higher-level instructions.
- While work is in progress, the reviewer checks on it with `panecom dump developer` roughly once an hour. If it looks stuck, send a question or the next task as appropriate. This guidance is not a timer that fires on its own while the AI is idle.

If you are asked to collaborate with someone in another project, set up `panecom link <alias> <path>` yourself from the repository path and alias you were given, and communicate as `<alias>/<role>`. Use `panecom link -f` only when the other side hasn't registered yet and you know they will join later. If you remove a temporary link when you are done, remove only the one you created for that work with `panecom unlink <alias>`. Never remove pre-existing links or links managed by a profile. If you can't identify the target project, ask for the path.

## Requests and replies (don't break the loop)

**A request that arrived through panecom is answered through panecom.** The other side is in a different pane and cannot see your screen. Writing the result in your own pane leaves them waiting.

- **If you received the request**: when the work is done, or you have reached a conclusion or decision, send `panecom tell <requesting role> "<result>"`. **The task is not complete until that message is sent.**
- **How to find the reply address**: a message delivered by `tell` starts with a header. There are two shapes, and **the presence of `project=` changes how you reply**.

  | Header received | Sender | How to reply |
  |---|---|---|
  | just the one line `[panecom:from=reviewer]` | same project | `panecom tell reviewer "<reply>"` |
  | a second line `[panecom:project=...]` | **another project** | `panecom link <alias> <project path>`, then `panecom tell <alias>/<the from role> "<reply>"` |

  ```
  [panecom:from=d]
  [panecom:project=/path/to/other project]
  please review this
  ```

  ↓ how to reply

  ```
  panecom link other "/path/to/other project"
  panecom tell other/d "<reply>"
  ```

  **Reading the header**: one value per line, everything between `[panecom:<key>=` and the `]` at end of line. **It is not split on whitespace** — project paths can contain spaces, so read to end of line as a single value. Quote it when passing it to a shell.

  **Never reply with the bare role name.** A role only means something inside its namespace, so replying to a cross-project `from=d` with `panecom tell d` looks for `d` in *your* project. If it isn't registered you get an error, and **if a different agent happens to share the name, the message goes to the wrong place**. Whenever a `[panecom:project=...]` line is present, go through a link (the alias is yours to choose; reuse an existing link if there is one).
- **What to put in the reply**: the conclusion (approve / changes requested / …), the reasoning, and what the other side should do next. If it gets long, send the key points and say where the details live (a PR comment, a file).
- **If you are the sender**: `tell` adds your own role automatically, so there is no need to introduce yourself in the body. `send` carries no sender, leaving the recipient unable to work out where to reply.

  ```
  panecom tell reviewer "please review PR #20: <URL>"
  ```

  What lands in their pane:

  ```
  [panecom:from=developer]
  please review PR #20: <URL>
  ```

  Send to someone in another project and your project path is added on line 2 so they can reply:

  ```
  [panecom:from=developer]
  [panecom:project=/path/to/myproject]
  please review PR #20: <URL>
  ```

- **It doesn't matter how the request reached you**: for collaborative work you are doing with another role, return the result to that role with `panecom tell` even when the user asked you directly inside your pane. The other side can't see your screen regardless of where the request came from. **This covers results of that collaboration only** — don't forward unrelated work the user merely happened to ask you for in the same pane.
- **Don't go quiet when you can't answer**: if you can't decide, or you lack information, send back *what is missing*.
- **When it takes a while**: send one message saying you've started before you dig in, so the other side knows whether to wait or re-ask.

A violation looks like: the reviewer prints the review in their own pane and stops; the developer keeps waiting; the user ends up copying the text across by hand. The same applies when that review began as a direct instruction from the user — it is the result of collaboration with the developer, so it goes back to them.

## Picking the multiplexer

panecom detects Zellij and tmux automatically. You can also be explicit:

```
panecom --mux tmux <command>       # --mux flag
PANECOM_MUX=tmux panecom <command> # environment variable
```

Precedence: `--mux` > `PANECOM_MUX` > auto-detection (`ZELLIJ_SESSION_NAME` > `TMUX`)

## Choosing between commands

### Things to run in your own shell (no panecom)

- File edits, builds, tests, git — ordinary development work
- Anything where you read the result yourself and decide what's next

### Things that use panecom (only when the user asked)

| Goal | Command |
|---|---|
| send a message to another AI agent | `panecom tell <role> "<message>"` |
| type a literal string into a pane (the other side may not be an AI) | `panecom send <role> "<text>"` |
| look at another AI agent's screen | `panecom dump <role>` |
| open a shared terminal | `panecom open terminal` |
| run a command in the shared terminal | `panecom send terminal "<command>"`, then `panecom dump terminal` for the result |
| show your screen to someone (for humans) | `panecom share <role>` |

## tell vs send (important)

**`tell` when the other side is an AI agent, `send` when you want to drive the pane directly.** When in doubt, use `tell`.

| | `tell` | `send` |
|---|---|---|
| Target | an AI agent holding a role (humans included) | any pane (shell, TUI, editor, …) |
| Payload | line 1 `[panecom:from=<your role>]` (plus line 2 `[panecom:project=<your path>]` when it's cross-project) | **exactly the string you gave** |
| Your own registration | **required** (error if unregistered) | not needed |
| Used for | requests, questions, reports, review results | typing commands, sending keys, answering prompts |

```
panecom tell reviewer "please review PR #20"   # to an agent
panecom send terminal "git status"             # typed straight into the pane
```

`send` is a general-purpose primitive and adds no header, because a stray `[panecom:from=...]` would corrupt input aimed at a shell or TUI. Using `send` for an agent leaves off the sender, so they can't work out where to reply — that is why `tell` exists separately.

When you send to another project (`<alias>/<role>`), line 2 of the header carries your project path, because role names only work inside a namespace and the recipient cannot reply from a role name alone. If your own project cannot be determined (broken registration, say), `tell` errors out instead of sending — better to stop than to send a message nobody can answer. For the receiving side, see the table in "Requests and replies".

An empty message is also refused. With no body, the header itself becomes the thing delivery verification looks at, and every message would look identical.

`tell` errors out when run from an unregistered pane. Its whole point is to state who you are, so run `panecom register <role>` first. If you only want to send a string without identifying yourself, use `send`.

Note that `send` only types — **nothing comes back**. When you run a command in a shell pane and need the result, `send` first and then read the screen with `panecom dump <role>`.

## Delivery verification (both tell and send, important)

`tell` and `send` both just type into the other side's screen and press Enter, so if the recipient is **not in input mode** the text is silently dropped.
panecom therefore checks their screen after sending (on by default) and exits non-zero when it cannot confirm delivery. This applies to both commands.

| Exit code | Meaning | What you should do |
|---|---|---|
| 0 | the screen changed after sending and the message is no longer in the input field = it can be treated as accepted | carry on |
| 1 | error: role not resolved, pane gone, … | check the configuration and the role. Do not resend |
| 3 | the screen did not change at all = **probably not accepted**; the recipient is in a detail view, pager or dialog and isn't taking input | read the screen on stderr and judge the state (below) |
| 4 | the message is still in the input field = Enter didn't take | check their state and resend if needed |

On a non-zero exit the last 20 lines of their screen go to stderr, so **read that first**.

### Judging exit 3 / 4

Infer the recipient's state from the screen on stderr. Typical cases:

| What you see | State | What to do |
|---|---|---|
| `Showing detailed transcript`, … | Claude is in the detail view | ask the user to press Ctrl+O to return to the input screen |
| `Do you want to...` / `1. Yes`, … | waiting on an approval dialog | **do not send keys yourself.** Ask the user to decide |
| `(END)` / `:` | a pager is open | ask the user to quit it with `q` |
| `-- INSERT --`, … | an editor is running | tell the user |
| looks like a normal prompt | they may be busy and rendering had stalled | wait a moment, re-check with `panecom dump <role>`, and resend once only if the request didn't arrive |

**What not to do**: sending Enter, Esc or Ctrl+C to "unstick" them when you can't tell what state they're in. A stray keypress on an approval dialog can authorise something nobody intended.

### Turning verification off

Only when you deliberately want to skip the check:

```
panecom tell --no-verify <role> "<message>"
panecom tell --verify-timeout 10 <role> "<message>"   # when they render slowly
panecom send --no-verify <role> "<text>"              # send takes the same flags
```

## Using the shared terminal

For working in a terminal together with the user.

1. Open the shared terminal:
   ```
   panecom open terminal
   ```
2. Type a command into it:
   ```
   panecom send terminal "kubectl get pods -A"
   ```
3. Read the result:
   ```
   panecom dump terminal
   ```

**Run ordinary commands in your own environment (the Bash tool).** Use the shared terminal only when one of these applies: the user needs to see it, it has to run with the user's environment or credentials, or it has to reuse the same shell state (cd, environment variables).

### Reading results

`send` only types, so **there is no mechanism that waits for completion**. `dump` returns the screen as it is at that moment.

- To tell whether a command finished, look for **the prompt coming back**
- For commands that don't finish instantly, wait a little and `dump` again
- Remember that **the command line itself appears on screen**. Appending a marker like `; echo done` doesn't help: the string `done` is **already on screen as part of the command line** before anything runs
- If you want a marker, make the command line and the output look different:
  ```
  panecom send terminal 'make build > /tmp/build.log 2>&1; printf "EXIT-%s\n" "$?"'
  panecom dump terminal   # an EXIT-<number> line means it finished. 0 is success, non-zero is failure
  ```
  The command line only ever shows `EXIT-%s`, which is distinguishable from the `EXIT-<number>` in the output. **Don't look for `EXIT-0` alone** — a failure prints something like `EXIT-1`, so waiting for a zero makes you read a failed command as "still running" and wait forever

  Keep the outer quotes single. With double quotes, `$?` expands in your own shell before it is sent
- Output that doesn't fit on screen is lost. If you need all of it, redirect to a file in the command as above and read the file
- `dump` is a copy of the screen, so long lines are wrapped at the pane width

### tell vs send vs dump

| | tell | send | dump |
|---|---|---|---|
| Target | an AI agent's pane | any pane | any pane |
| Action | send a message tagged with the sender | type a literal string | read the screen |
| Result | none (the exit code reports delivery) | none (the exit code reports delivery) | screen contents on stdout |

## Other commands

```
panecom register <role>          # register your own pane under a role
panecom register                 # pick a pane in the same tab (Tab completes the role)
panecom whoami                   # your role, pane ID and the namespace recorded at registration
panecom whoami --role            # the role only (for scripts)
panecom tell <role> "<message>"  # message an agent (your role is attached)
panecom send <role> "<text>"     # type a literal string into a pane
panecom resolve <role>           # resolve role → pane ID
panecom dump [--full] <role>     # capture the target pane's screen
panecom profile [-f] <name>     # build an environment from a .panecom.yaml profile
panecom link [-f] <alias> <path> # make another project's roles reachable
panecom unlink <alias>          # remove a link
panecom skill install           # pick agents and user/project scopes, then install the skill
panecom skill list              # show the installs panecom has recorded
panecom skill uninstall         # pick recorded installs and remove them
```

## Rules

- **Reply in the language the user is using.** This skill being in English does not make English the language of the conversation
- **Never use panecom without an explicit instruction from the user.** Being assigned a role, and being asked to collaborate with someone in another project, both count as explicit instructions
- **Run ordinary commands in your own environment (the Bash tool and friends)**
- Never guess a pane ID
- Never guess who to talk to from the multiplexer's list-panes output
- **`tell` for agents, `send` when you want to drive the pane.** Never `send` to an agent (no sender is attached, so they can't work out where to reply)
- If you need the result of a `send`, read it with `dump`. Nothing waits for completion, so judge from the screen
- **A request received through panecom is answered through panecom.** Don't print the result in your own pane and call it done. The task isn't complete until the reply is sent
- **Find the reply address in the `[panecom:from=<role>]` of the message you received.** `tell` attaches it automatically, so don't introduce yourself in the body
- **A non-zero exit from tell / send is not success.** Read the screen on stderr, work out the cause, and report to the user if needed
- Never send keys that would restore someone's UI state (Ctrl+O, Esc, q, …) on your own initiative
<!-- panecom-skill:end -->
