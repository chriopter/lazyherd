# lazyherd

A cockpit over every Git repository in your [Herdr](https://herdr.dev)
workspace. See which repos have uncommitted changes, which are ahead or
behind, and press Enter to open [lazygit](https://github.com/jesseduffield/lazygit)
in the one you pick. Press Esc to get back from lazygit to the overview.

![lazyherd](docs/screenshot.png)

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
source needs Go 1.24 or newer. `git` and `lazygit` must be on your `PATH`.

## Use

```sh
lazyherd            # scans ~/git
lazyherd ~/code     # or any directory of repos
```

Short on typing? `alias lh=lazyherd` in your shell rc.

| Key | Action |
|-----|--------|
| `↵` | Open lazygit in the selected repo |
| `c` | Stage everything and commit, asking for a message |
| `p` | `git pull --ff-only` in the selected repo |
| `P` | `git push` in the selected repo |
| `f` | `git fetch --all` in the selected repo |
| `F` | Fetch all repos |
| `R` | Rescan |
| `/` | Filter by name |
| `t` | Jump to the repo's [Herdr](https://herdr.dev) tab, or open one |
| `w` | Toggle between the current Herdr workspace and all repos |
| `q` | Quit |

Only the immediate subdirectories of DIR are scanned; symlinked directories
are skipped. Repos are sorted
dirty first, then out of sync, then by name. Ahead/behind counts come from
the local tracking refs, so they are only as fresh as the last fetch; `F`
runs `git fetch --all` in every scanned repo, including the ones hidden by
a filter.

To make `Esc` leave lazygit and return to the cockpit, add to
`~/.config/lazygit/config.yml`:

```yaml
quitOnTopLevelReturn: true
```

## Herdr

Started inside a Herdr pane, lazyherd shows only the repos that have a pane
in the current workspace. `w` switches to all repos, `t` focuses the tab that
already works in the selected repo or creates one. Without Herdr both keys
are hidden and everything else works as usual.

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
