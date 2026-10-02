# ngt (ng-dev-tools)

A personal toolbox of developer utilities for macOS, written in Go.

## Install

```sh
make install        # installs `ngt` into $(go env GOPATH)/bin
# or
make build          # builds ./bin/ngt
```

Requires Go 1.27+.

## Commands

### `ngt clean`

Scans your Mac for reclaimable disk space, lets you pick what to remove in an interactive list, then **permanently deletes** it (nothing goes to the Trash).

```sh
ngt clean                                  # scan everything
ngt clean --dry-run                        # full flow, deletes nothing
ngt clean --category dev --older-than 30d  # only developer junk, 30-day staleness
ngt clean --projects-dir ~/work            # where to look for stale node_modules, target/, Pods/...
ngt clean | less                           # non-interactive report when not a terminal
```

| Category | What it finds |
|----------|---------------|
| `caches` | `~/Library/Caches`, logs, browser and Electron app caches, Apple service caches, `/Library/Caches` and `/Library/Logs` (root), Trash, iOS updates and device backups |
| `dev`    | Xcode DerivedData, archives, device support, simulators and runtimes (`simctl`), npm/Yarn/pnpm/Bun, Go, Gradle, Maven, CocoaPods, SwiftPM, pip/uv/Poetry, Cargo, Homebrew, Docker, JetBrains, stale project build folders |
| `apps`   | Apps not opened within the staleness window (with their support files) and leftovers of uninstalled apps |
| `files`  | Large files and old installers in `~/Downloads` and `~/Desktop` |

Items tagged **review** may contain something you want to keep and are not selected by default. Items tagged **sudo** are root-owned; if you select any, you are asked for your password once before cleaning starts. Every run writes a log to `~/Library/Logs/ngt/`.

Keys: `↑/↓` move, `space` toggle (on a header: the whole category), `c` category, `a` all, `n` none, `enter` continue, `q` quit.

Safety: every path is checked right before deletion and must be strictly inside `$HOME`, `/Applications`, `/Library/Caches` or `/Library/Logs`; well-known folders themselves (`~/Documents`, `~/Library`, ...) and sensitive ones (`~/.ssh`, Keychains, iCloud Drive, Mail) are always refused. Symlinks are never followed.

Some locations (Trash, Mail) need Full Disk Access for your terminal app; without it they are skipped and listed at the end.

### `ngt ports`

Shows every process listening on a TCP port, with its project (the git repository it was started from), uptime and addresses, and stops the ones you pick.

```sh
ngt ports                  # interactive list
ngt ports 3000             # stop whatever holds :3000, after a y/N prompt
ngt ports 3000 8080 -y     # no prompt
ngt ports 5173 --force     # SIGKILL straight away
ngt ports --all            # include macOS system and simulator processes
ngt ports | grep node      # plain table when not a terminal
```

Processes get SIGTERM, then SIGKILL if they are still running after `--grace` (default 3s). Right before signalling, ngt checks that the pid still belongs to the same program and start time, so a recycled pid is never hit. Processes that need care are tagged: `docker` (Docker Desktop, which owns every published container port), `airplay` (AirPlay Receiver on 5000/7000), `system` and `simulator`.

Keys: `↑/↓` move, `space` toggle, `a` all, `n` none, `enter` stop the checked processes (or the one under the cursor), `r` refresh, `q` quit.

### `ngt doctor`

Checks that this Mac is ready for Node, React Native (iOS and Android), Expo and Go work, and prints the command that fixes each problem. It only reads; it never installs or changes anything.

```sh
ngt doctor                 # every check, with a progress bar
ngt doctor android go      # only some groups
ngt doctor --problems      # hide what passed
ngt doctor --offline       # skip network checks and latest-version lookups
```

Groups: `node` (nvm, Node on the latest LTS, pnpm 11+), `ios` (Xcode, simulator runtime, CocoaPods, Ruby, Watchman, EAS), `android` (JDK 17, JAVA_HOME, ANDROID_HOME, SDK components, AVD), `go` (Go and gopls, dlv, golangci-lint, gofumpt, goimports), `claude`, `shell` (UTF-8 locale, PATH, competing Node installs, open files limit), `git` (identity, defaults, gh, GitHub SSH), `globals` (misplaced and outdated npm globals, Homebrew), `network` (DNS, the registries and CDNs you download from, proxies), `services` (Docker, brew services, failing launch agents), `caches` (build caches over a size limit), `system` (disk, memory, FileVault, firewall, SIP, Time Machine, macOS updates, uptime, heat, battery, kernel panics, clock).

Run it from your normal terminal so it sees the same environment variables as your builds. It exits with status 1 when any check fails. To add a check, add a `Check` to the group's file in `internal/doctor/`.

### `ngt open`

Lists the projects in `~/projects`, most recently opened or changed first, with their git branch, a `●` for uncommitted changes and when you last worked on them. Type to filter, press `enter`, and pick the IDE in the select box. Each project remembers its own IDE: next time the box starts on the IDE that project was last opened in (tagged `last used`), and a project you have not opened yet starts on the IDE you picked most recently (tagged `default`).

```sh
ngt open                   # pick a project, then an IDE
ngt open museum            # start filtered; a single match goes straight to the IDE box
ngt open --root ~/work     # another projects folder
ngt open | grep weather    # plain list, opens nothing
```

IDEs are found by their app bundle in `/Applications` and `~/Applications`: VS Code, VSCodium, Cursor, Windsurf, Zed, WebStorm, GoLand, IntelliJ IDEA, PyCharm, Rider, Android Studio, Xcode, Sublime Text and Nova. Xcode opens the project's `.xcworkspace` or `.xcodeproj` (including a React Native app's `ios/` one), and Android Studio opens a React Native app's `android/` folder.

A folder that only groups other folders (no `.git` and no files of its own) is replaced by the projects inside it, and empty folders are hidden. "Changed" is the newest file in the project, skipping `node_modules`, build output and hidden folders, or the last commit or checkout. The IDE for each project, your most recent pick and when you opened each project are kept in `~/.config/ngt/open.json`.

Keys: type to filter, `↑/↓` move, `enter` choose, `esc` clear the filter or quit. In the IDE box: `↑/↓` or `1`-`9`, `enter` open, `esc` back.

### `ngt status`

Shows the git state of every repository in `~/projects` on one screen, the ones needing attention first: a merge or rebase left in progress (`✖`), a branch behind its remote (`⇣`), uncommitted changes (`●`), then commits not pushed yet (`⇡`). Repositories with only stashes or other branches worth a look get a `◦`, and clean ones a `✓`.

```sh
ngt status                 # interactive list
ngt status --fetch         # git fetch everything first, to know what is behind
ngt status | grep -v clean # plain table when not a terminal
```

Columns: changes as `+staged ~modified ?untracked !conflicted`, sync with the upstream (`⇡2 ⇣1`, `not pushed`, `remote gone`, `no remote`), the last commit's age, and notes (stashes, other branches, the operation in progress). The details box below lists the changed files, the unpushed commits, the stashes and the other local branches that are behind, ahead, never pushed or whose remote branch was deleted, and says when the repository was last fetched.

It only reads (with `--no-optional-locks`, so looking never rewrites the index) and never contacts a remote unless asked. `--fetch`, `f` and `F` run `git fetch`, which updates remote-tracking branches and nothing else: no pull, merge or prune. A fetch that would need a password or passphrase fails instead of prompting. Enter opens the repository in its IDE with the same box as `ngt open`, and remembers the choice.

Keys: `↑/↓` move, `enter` open in IDE, `f` fetch the selected repository, `F` fetch all, `t` open the branch's Jira ticket, `r` refresh, `q` quit.

### `ngt prs`

Your open pull requests and the ones waiting for your review, across every repository on GitHub, in one list: review requests on top, then your own. Each row says what the pull request is waiting for (`conflicts`, `checks failing`, `changes requested`, `draft`, `checks running`, `ready to merge`, or who it is waiting on), its CI counts, size and last update. The details box lists every check, each reviewer's verdict and whether the branch conflicts with its base.

```sh
ngt prs                  # interactive list (alias: ngt pr)
ngt prs | grep conflicts # plain list when not a terminal
```

It reads GitHub with one GraphQL request through the `gh` CLI, so it uses your `gh auth login`.

- `enter` opens the pull request in the browser.
- `c` checks out its branch in your local clone, found in `~/projects` by its GitHub remote (`--root` for another folder). It runs `gh pr checkout`, which also fetches branches from forks, and refuses while the clone has uncommitted changes. When the repository is not cloned yet, it first clones it into `~/projects/<name>` with `gh repo clone` (the help line then says `c clone`), refusing a folder that already exists.
- `i` checks out the pull request's branch, like `c`, then opens the local clone in its IDE, with the same box as `ngt open`. It skips the checkout when the clone is already on that branch, and opens nothing when the checkout is refused.
- `l` shows the log of the failed GitHub Actions steps, scrolled to the end; `tab` moves to the next failed check and `o` opens the job page. GitHub keeps these logs for 90 days.

Keys: `↑/↓` move, `enter` browser, `c` check out (or clone), `i` IDE, `l` failed log, `t` open the Jira ticket in the title or branch, `r` refresh, `q` quit.

### `ngt env`

Compares the env files of every project in `~/projects` with the example that documents them (`.env.example`, `.env.sample`, `.env.template`), folder by folder, monorepo apps included. Only key names are shown, never values.

```sh
ngt env                # interactive list
ngt env | grep tracked # plain table when not a terminal
```

Rows come most urgent first: a local env file that git tracks, or that was committed and deleted but is still in history (`✖`, rotate its secrets), keys your files are missing (`●`), keys left empty (`○`), then keys the example does not list or that the code reads (`process.env.X`, `import.meta.env.X`, `os.Getenv`, `ENV["X"]`, ...) without the example naming them (`◆`). Folders with an example but no local file yet, or a local file with no example, get a `◦`.

Each example goes with the local files it describes: `.env.example` with `.env`, `.env.local` and `.env.production.local`, and `.env.local.example` with `.env.local` when both exist. A key counts as set when any of them sets it. Shared files committed on purpose, such as fastlane's `.env.default`, provide keys but are never flagged as committed. Env files that are named pipes, as 1Password Environments makes, are listed but never opened, since reading one hands over its secrets.

`a` appends the missing keys to the local file as `KEY=`, with the comments the example has above them, after asking. It is the only write, and it never changes a line already there.

Keys: `↑/↓` move, `a` add missing keys, `r` refresh, `q` quit.

### `ngt todo`

The `TODO`, `FIXME` and `HACK` comments in every git repository in `~/projects`, dated with `git blame` and listed oldest first, so the longest forgotten lead. Lines not committed yet come last.

```sh
ngt todo                # interactive list (alias: ngt todos)
ngt todo --mine | cat   # plain list of the lines you wrote
```

Only markers that start a comment count, in the comment syntax of the file's language, so `id: 'TODO'`, a `// TODO` inside a string or a comment that merely mentions TODO are left out. Tool directives may come first, as in `// @ts-expect-error FIXME`. In Markdown the marker must start the line (`> **TODO**: ...`). It searches tracked files and untracked ones git does not ignore, and skips dependencies, build output, lockfiles, binary files, files over 1 MB and lines over 400 characters. At most 6 git processes run at once, however many repositories there are.

The details box shows who added the line, when and in which commit, with the code around it.

- `enter` opens the file at that line in your IDE, with the same box as `ngt open` (Cursor, VS Code, VSCodium, Windsurf, Zed, Sublime Text, Xcode and JetBrains IDEs jump to the line; others open the file).
- `o` opens the commit that added the line on GitHub, once it is pushed.
- `m` shows only the lines you wrote: lines not committed yet, and lines whose author has any `user.email` or `user.name` set for that repository (global ones included), or your GitHub account (profile name, or a `users.noreply.github.com` address), so squash merges made on GitHub count too. Any email one of your lines carries then makes other lines with that email yours.

Keys: `↑/↓` move, `enter` open in IDE, `o` open commit, `t` open the Jira ticket in the note or its commit, `m` mine only, `r` refresh, `q` quit.

### `ngt standup`

What you did since the last day you worked, across `~/projects` and GitHub: your commits by project, under the pull request or branch they belong to, the pull requests you opened or merged, the others' ones you reviewed or commented on, and the work still in progress.

```sh
ngt standup                     # since the last day you worked
ngt standup --since monday      # the week so far
ngt standup --since 3d | pbcopy # plain text, to paste somewhere
```

The last day you worked is the most recent day before today with a commit of yours, so weekends and holidays are skipped (with none in the last 30 days, it is the previous weekday). `--since` also takes `today`, `yesterday`, a weekday, a date such as `2026-09-28`, or `3d` and `2w`.

- A commit is yours when its author is the `user.email` of its repository, on any local or remote branch, dated by when you wrote it, so a rebase does not bring old work back. Merge commits are left out. Worktrees and clones of one repository count once.
- A commit goes under the pull request that has it, or under the open pull request for its branch when it is not pushed yet; otherwise under its branch (`not pushed` or `no PR`), or the default branch.
- Pull requests with no commits in that time still show when they were opened, merged or closed then. Repositories you have no clone of are listed by their GitHub name.
- `In progress` lists the repositories with uncommitted changes or commits not pushed yet, as `ngt status` sees them.
- GitHub is read through `gh`. Without it, or when it is not logged in, the report only has local work and the title says why.

Keys: `↑/↓` move, `enter`/`o` open the pull request, commit or branch on GitHub, `[` start a day earlier, `]` a day later, `t` open the Jira ticket in the row, `r` refresh, `q` quit.

### `ngt claude`

Lists the projects in `~/projects`, the ones you last had a Claude Code session in first, with their git branch, a `●` for uncommitted changes and when you last worked on them. Type to filter and press `enter`: ngt is replaced by `claude`, started in the project's folder, as if you had typed `cd` and `claude` yourself.

```sh
ngt claude                 # pick a project, start a new session in it
ngt claude museum          # start filtered; a single match starts straight away
ngt claude --root ~/work   # another projects folder
ngt claude | grep weather  # plain list, starts nothing
```

A session counts for a project when it was started in the project's own folder, so one started in a monorepo's `apps/native` does not move the monorepo up. Only the transcripts' modification times are read, from `~/.claude/projects` (or `$CLAUDE_CONFIG_DIR/projects`), so the list opens as fast as `ngt open`. To continue an earlier session instead, use `ngt claude sessions`.

Keys: type to filter, `↑/↓` move, `enter` start claude, `esc` clear the filter or quit.

### `ngt claude sessions`

Every saved Claude Code session, from every folder, in one list, most recently active first: the first prompt, when it was last active, its status, the git branch, how many prompts it has and the model that wrote most of it. `claude --resume` only lists the current folder's sessions; this lists them all and searches what was said in them.

```sh
ngt claude sessions                  # interactive list
ngt claude sessions expo upgrade     # start with a search
ngt claude sessions | grep memorit   # plain table, with session ids, when not a terminal
```

The status shows what Claude is doing with a session that is open right now, in a terminal, the Claude app or an editor: `working` while it runs a turn, `waiting` when it needs you (such as a permission prompt) and `idle` once it has finished its turn. It is read from the status file each running Claude Code keeps in `~/.claude/sessions` and refreshes every 2 seconds; a closed session has none.

The tokens are what the API reported for each reply, subagents' included: the context size at the latest reply (how full the conversation is, which decides when it compacts), then the input, output, cache reads and cache writes added up over the session. While a session is open its counts follow its transcript, reading only what was added since the last refresh.

Typing searches your prompts, Claude's replies and each session's title, folder, branches and models; every word must appear, in either case. The details box shows the session's title and status, its folder, every branch it was on and its id; then when it started and was last active, its size, the replies per model and the tokens it used, beside the pull requests it opened; then its first and last prompts or, while searching, the line that matched. Each pull request is looked up on GitHub through `gh` while the list loads, in one request per 50, and shows whether it is open, a draft, merged or closed, the latest first. Below 100 columns the pull requests fit on one line under the other facts.

`enter` replaces ngt with `claude --resume <id>` in the folder the session belongs to, so it continues where it was saved. A folder that was moved or deleted is struck through and cannot be resumed. Slash commands, shell commands, background task notices and subagents' transcripts do not count as prompts; a session without any prompt is left out.

Sessions are read from `~/.claude/projects`, or `$CLAUDE_CONFIG_DIR/projects`. It only reads them, 4 at a time, and skips transcript lines over 16 MB, which hold tool output such as screenshots.

Keys: type to search, `↑/↓` move, `enter` resume, `ctrl+t` open the Jira ticket in the title, branch or first prompt, `esc` clear the search or quit.

### `ngt settings`

Shows and changes ngt's settings, kept in `~/.config/ngt/settings.json`. You can also edit that file by hand: it uses the same names (`{"jiraHost": "acme.atlassian.net"}`), every command reads it when it starts, and a misspelt name or an invalid value is reported and ignored.

```sh
ngt settings                                  # every setting, its value and what it does
ngt settings set projectsDir ~/work
ngt settings set jiraHost acme.atlassian.net
ngt settings unset jiraHost                   # back to the default
cd "$(ngt settings get projectsDir)"
```

- `projectsDir` is the folder that holds your projects, `~/projects` by default. `open`, `run`, `status`, `prs`, `env`, `todo`, `standup`, `claude` and `claude sessions` read it, and `--root` still overrides it for one run. `clean` searches it for stale build folders alongside `~/Developer`, `~/code`, `~/src` and `~/workspace`. It must be an existing folder; a relative path or a quoted `~` is resolved when you set it.
- `jiraHost` is your Jira site. Ticket keys such as `TS-234455` or `COREX-344` in branch names, commit subjects, pull request titles, session titles and TODO notes become underlined links to `https://<jiraHost>/browse/<key>`, clickable in terminals that support links (cmd+click in iTerm2, Ghostty, WezTerm, Kitty or Warp). Terminal.app shows them as plain text, and Wave asks to open them but then does not; in any terminal, `t` (`ctrl+t` in `claude sessions`) opens the selected row's ticket in your browser. It takes a host or any address on the site, such as a ticket's page, and keeps a folder Jira is served from (`jira.example.com/jira`). A key is any uppercase project key, a dash and a number, so the odd `UTF-8` gets a link too.

## Output for scripts and AI agents

Every command takes `--json`. It never opens the interactive screen or asks anything, even in a terminal, and prints one JSON object to stdout with everything the screen shows, details box included. Warnings go to stderr, and the exit status is the same as without it (`ngt doctor` still exits 1 when a check fails).

```sh
ngt status --json | jq '.repos[] | select(.behind > 0) | .name'
ngt prs --json | jq '.mine[] | {url, state, checkCounts}'
ngt doctor --problems --json
ngt claude sessions expo --json | jq '.sessions[0].tokens'
ngt ports 3000 --yes --json      # stopping needs --yes, since nothing can be asked
```

- Keys are camelCase. Times are RFC 3339, sizes are bytes (`sizeBytes`) and durations milliseconds (`durationMs`). Paths are absolute.
- Empty lists, unset times and empty optional strings are left out; the command's main list is always an array, even when empty.
- ngt's own states are lowercase words (`"checks failing"`, `"rebase"`, `"already-exited"`); GitHub's (`reviewDecision`, `mergeable`, a pull request's `state`) are spelled as GitHub returns them.
- It only reads: nothing is deleted, opened, run or resumed. The one exception is `ngt ports <port...> --yes --json`, whose job is stopping.

Without `--json`, piped output stays the plain table described under each command.

## Development

```sh
make test   # go test -race ./...
make lint   # golangci-lint
make fmt
```

Layout:

```
cmd/ngt/            entry point
internal/cli/           cobra commands and flags; add new features here
internal/cleanup/       clean domain: items, scanner, cleaner, deletion guard
internal/cleanup/rules/ the catalogue of things to clean (one file per area)
internal/cleanup/tui/   Bubble Tea screens for `clean`
internal/ports/         ports domain: listing listeners (lsof, ps) and stopping processes
internal/ports/tui/     Bubble Tea screens for `ports`
internal/doctor/        doctor domain: the check catalog (one file per group) and the concurrent runner
internal/doctor/tui/    progress screen and report for `doctor`
internal/projects/      open domain: finding projects, recent activity, git branch and status, saved choices
internal/projects/tui/  project list and IDE select box for `open`, also the project picker for `claude`
internal/ide/           detecting installed IDEs and opening projects, or a file at a line, in them
internal/ide/idepicker/ the IDE select box shared by `open`, `status`, `prs` and `todo`
internal/gitstatus/     status domain: reading each repository's changes, sync, stashes, branches; fetch
internal/gitstatus/tui/ table and details box for `status`
internal/pulls/         prs domain: reading pull requests through gh, local clones, checkout, failed logs
internal/pulls/tui/     grouped list, details box and log view for `prs`
internal/envfiles/      env domain: env file names and keys, examples vs local files, git exposure, keys read in code
internal/envfiles/tui/  table, details box and add-missing prompt for `env`
internal/todos/         todo domain: finding marker comments in the files git lists, dating them with git blame
internal/todos/tui/     table and details box with the surrounding code for `todo`
internal/standup/       standup domain: your commits by branch, your pull request activity through gh, the last day you worked
internal/standup/tui/   grouped report for `standup`, also printed as plain text
internal/claudesessions/ claude domain: reading Claude Code transcripts, searching them, resuming one, starting a new one
internal/claudesessions/tui/ searchable table and details box for `claude sessions`
internal/ui/            shared styles and widgets (progress panel, list cursor, row highlight)
internal/fsx/           filesystem helpers (disk usage, removal)
internal/macos/         wrappers for mdls, simctl, ps, Info.plist
```

To add a cleanup target, add a `PathRule` (or a small `cleanup.Rule` implementation) to the relevant file in `internal/cleanup/rules/`. To add a new command, create `internal/cli/<name>.go` and register it in `NewRootCmd`.
