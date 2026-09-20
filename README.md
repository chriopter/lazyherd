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

Or grab a binary from the [releases](https://github.com/chriopter/lazyherd/releases).
lazygit must be on your `PATH`.

## Use

```sh
lazyherd            # scans ~/git
lazyherd ~/code     # or any directory of repos
```

| Key | Action |
|-----|--------|
| `↵` | Open lazygit in the selected repo |
| `/` | Filter by name |
| `f` | `git fetch` in all repos |
| `r` | Rescan |
| `t` | Jump to the repo's [Herdr](https://herdr.dev) tab, or open one |
| `w` | Toggle between the current Herdr workspace and all repos |
| `q` | Quit |

Repos are sorted dirty first, then out of sync, then by name.

To make `Esc` leave lazygit and return to the cockpit, add to
`~/.config/lazygit/config.yml`:

```yaml
quitOnTopLevelReturn: true
```

## Herdr

Started inside a Herdr pane, lazyherd shows only the repos that have a pane
in the current workspace. `w` switches to all repos, `t` focuses the tab that
already works in the selected repo or creates one. Without Herdr both keys
simply do nothing.

## License

MIT
