# lazyherd

A cockpit over every Git repository in your [Herdr](https://herdr.dev)
workspace. See which repos have uncommitted changes, which are ahead or
behind, and press Enter to open [lazygit](https://github.com/jesseduffield/lazygit)
in the one you pick. Press Esc to get back from lazygit to the overview.

![lazyherd](docs/screenshot.png)

## Features

- Scans every Git repository directly under one directory, in parallel.
- Repo list with number of changed files, branch, ahead/behind counts; dirty repos first.
- Change tree of the selected repo with git status codes per file.
- Diff of the selected file, or the recent log when the repo is clean.
- Enter opens lazygit in the selected repo; on exit the list is rescanned.
- Commit dialog with subject and description; stages everything. Ctrl+G lets Claude Code write both.
- Pull (`--ff-only`) and push for the selected repo.
- Refreshes on its own: status every 3 seconds, `git fetch --all` in every repo once a minute.
- Name filter.
- Keyboard and mouse selection.
- Herdr: started inside a workspace, groups that workspace's repos first and marks them with ⌂; Space pins any other repo to the workspace (★, remembered in `~/.config/lazyherd/pins.json`); `w` narrows the list to them; jump to a repo's tab or open one.
- Light and dark terminal themes.

## Install

With [mise](https://mise.jdx.dev) (updates via `mise upgrade`):

```sh
mise use -g ubi:chriopter/lazyherd
```

With Go:

```sh
go install github.com/chriopter/lazyherd@latest
```

Or grab a Linux or macOS binary from the
[releases](https://github.com/chriopter/lazyherd/releases). Building from
source needs Go 1.24 or newer. Requires `git` and `lazygit` on your `PATH`;
`claude` for generated commit messages.

## Use

```sh
lazyherd            # scans ~/git
lazyherd ~/code     # or any directory of repos
```

`alias lh=lazyherd` in your shell rc for the short name.

| Key | Action |
|-----|--------|
| `↵` | Open lazygit in the selected repo |
| `l` | Move into the file tree; `j`/`k` pick a file, `esc` goes back |
| `c` | Commit dialog: subject and description, `tab` switches fields, `ctrl+g` writes both with `claude -p`, `enter` commits (`alt+enter` from the description) |
| `p` | `git pull --ff-only` in the selected repo |
| `P` | `git push` in the selected repo |
| `/` | Filter by name |
| `t` | Jump to the repo's Herdr tab, or open one |
| `w` | Toggle between the current Herdr workspace and all repos |
| `space` | Pin the selected repo to the current Herdr workspace, or unpin it |
| `q` | Quit |

Notes:

- Only the immediate subdirectories of the scanned directory are considered; symlinked directories are skipped.
- Ahead/behind counts come from the local tracking refs; the background fetch keeps them at most a minute old.
- Generated commit messages run `claude -p` inside the repo, so its `CLAUDE.md` conventions apply. The diff sent is capped at 60 kB.
- Without Herdr, `t` and `w` are hidden and everything else works as usual.

For `Esc` to leave lazygit and return to the cockpit, add to
`~/.config/lazygit/config.yml`:

```yaml
quitOnTopLevelReturn: true
```

## Development

```sh
make test    # go test ./...
make lint    # gofmt and go vet
make build   # ./lazyherd
```

Releases are cut by pushing a `v*` tag; GitHub Actions runs the tests and
goreleaser publishes the binaries.

## License

MIT
