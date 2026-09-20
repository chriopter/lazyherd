# lazyherd

A cockpit over every Git repository in your [Herdr](https://herdr.dev)
workspace. A narrow list shows which repos have uncommitted changes and
which are ahead or behind; the repo you select is opened in
[lazygit](https://github.com/jesseduffield/lazygit) in a pane next to it.

![lazyherd](docs/screenshot.png)

## Features

- Scans every Git repository directly under one directory, one `git status` call each, in parallel.
- Repo list with number of changed files, branch, ahead/behind counts; dirty repos first.
- Inside Herdr: splits a pane to the right and keeps lazygit open there for the selected repo. Moving the selection switches the repo.
- Outside Herdr: Enter opens lazygit full screen; quitting it returns to the list.
- Sync: fast-forward pull, then push when ahead; for the selected repo or every listed repo.
- Refreshes on its own: status every 3 seconds, `git fetch --all` in every repo once a minute.
- Name filter.
- Herdr workspaces: repos with a pane in the current workspace come first, marked ⌂; Space pins any other repo to the workspace (★); `w` narrows the list to them; `t` jumps to a repo's tab or opens one.
- Keyboard and mouse selection, light and dark terminal themes.

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
source needs Go 1.24 or newer. Requires `git` and `lazygit` on your `PATH`.

## Use

```sh
lazyherd            # scans ~/git
lazyherd ~/code     # or any directory of repos
```

`alias lh=lazyherd` in your shell rc for the short name.

| Key | Action |
|-----|--------|
| `↵` | Focus the lazygit pane (Herdr), or open lazygit full screen |
| `p` | Sync the selected repo: `git pull --ff-only`, then `git push` if ahead |
| `P` | Sync every listed repo |
| `/` | Filter by name |
| `t` | Jump to the repo's Herdr tab, or open one |
| `w` | Toggle between the current Herdr workspace and all repos |
| `space` | Pin the selected repo to the current Herdr workspace, or unpin it |
| `q` | Quit, closing the lazygit pane |

Notes:

- Only the immediate subdirectories of the scanned directory are considered; symlinked directories are skipped.
- Ahead/behind counts come from the local tracking refs; the background fetch keeps them at most a minute old.
- The lazygit pane is a Herdr pane running `lazyherd follow <socket>`, which starts `lazygit -p <repo>` for each selection. Quit lazygit with `q` and the next selection starts it again.
- Pins are stored in `~/.config/lazyherd/pins.json`, keyed by workspace name.
- Without Herdr, `t`, `w` and Space are hidden and everything else works as usual.

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
