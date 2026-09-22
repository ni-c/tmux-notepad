# tmux-notepad

<!-- badges: start -->
[![CI](https://github.com/ni-c/tmux-notepad/actions/workflows/ci.yml/badge.svg)](https://github.com/ni-c/tmux-notepad/actions/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/ni-c/tmux-notepad/badge)](https://scorecard.dev/viewer/?uri=github.com/ni-c/tmux-notepad)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/14755/badge)](https://www.bestpractices.dev/projects/14755)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
<!-- badges: end -->

Prompt queueing for coding agents in tmux — and a text queue for everything
else in the terminal.

Press a key, a popup shows the entries of a Markdown note, pick one, and its
text lands in the input line of the pane you came from. **It never presses
Enter** — you do.

That is the whole trick, and the reason the same queue is safe for a coding
agent and for a root shell.

```
┌─────────────────────────────┬──────────────────────────────────────┐
│ Release Prompts                                       → %1 (zsh)   │
│ ▸ Release notes       │ Go through the merged pull requests since  │
│   ✓ Bump version      │ the last tag and write the changelog       │
│   make test           │ entries, grouped by added, fixed, changed. │
│   Review checklist    │                                            │
│ enter paste  tab paste·stay  / search  f note  ? keys  q close     │
└─────────────────────────────┴──────────────────────────────────────┘
```

## What it is for

**Queueing prompts.** Write down the next five things you want your agent to
do while it is still working on the first, then send them one at a time — each
one only after you have seen what the last one did. Claude Code, Codex,
OpenCode, Kimi Code, and whatever comes next: tmux-notepad has no idea what
runs in the target pane, so there is nothing to adapt per tool.

**And any other text you retype too often.** The migration command with six
flags, the `psql` invocation for the staging database, an `ssh` one-liner, the
`curl` you need again every time the token expires. It is a text queue for the
terminal; prompts are only the case that hurts most without one.

## Why a note file

The text you want next tends to live somewhere else — a notes app, a wiki, a
file on a share. Getting it into a terminal means switching windows, finding
it, selecting it, copying, switching back. This keeps it one key away, and
ticks off what you have already sent.

Any directory of Markdown files works, including one a web-based notes app
writes to.

## Note format

An entry is a top-level heading plus the text below it:

```markdown
# Release notes

Go through the merged pull requests since the last tag and write the
changelog entries, grouped by added, fixed and changed.

# make test

# Review checklist

- run the linter
```

- The **heading is a label**; what gets pasted is the text below it. An entry
  with no text pastes its own title, so `# make test` works as a one-liner.
- `##` and deeper belong to the entry, so long prompts can have structure.
- Fenced code blocks are skipped, so a `# comment` in a shell snippet does not
  become an entry.
- Once pasted, an entry is ticked off in the file as `# ✓ Release notes` —
  visible in every editor and in whatever web UI writes these files.

## Install

One command, no Go needed:

```sh
curl -fsSL https://github.com/ni-c/tmux-notepad/releases/latest/download/install.sh | bash
```

That picks the binary for your system, checks it against a checksum baked into
the installer, puts it in `~/.local/bin`, writes a starter config if you have
none, and binds `prefix` + <kbd>n</kbd> in `~/.tmux.conf`. Pass `--no-bind` to
keep your config to yourself, `--prefix DIR` to install somewhere else, and
`--uninstall` to take it back out — your notes and config stay.

With [tpm](https://github.com/tmux-plugins/tpm), one line in `~/.tmux.conf`:

```tmux
set -g @plugin 'ni-c/tmux-notepad'
```

`prefix` + <kbd>I</kbd> installs the plugin, and the binary is fetched the first
time you press the key — inside the popup, where you can see it happen. tmux
itself never waits on the network.

From source, if you have Go:

```sh
go install github.com/ni-c/tmux-notepad/cmd/tmux-notepad@latest
```

Or `make install` from a checkout, which also goes to `~/.local/bin`.

Binding it by hand works too:

```tmux
bind n display-popup -E -w 80% -h 80% -T ' notepad ' 'tmux-notepad'
```

`display-popup` is used directly on purpose. `run-shell -b` has no client and
cannot show a popup at all; without `-b` it blocks the client while the popup
is open. The target pane is resolved by tmux-notepad itself.

### Verifying a download

Every release is built by a workflow in this repository and carries signed
provenance. With the [GitHub CLI](https://cli.github.com):

```sh
gh attestation verify install.sh --repo ni-c/tmux-notepad
```

Each release also ships `tmux-notepad.intoto.jsonl`, so the check works without
GitHub's attestation API:

```sh
gh attestation verify install.sh --bundle tmux-notepad.intoto.jsonl \
  --repo ni-c/tmux-notepad
```

`SHA256SUMS` covers every asset for a check by hand. What the two install paths
ask you to trust differs a little; [SECURITY.md](SECURITY.md) spells it out.

## Configure

Copy [`config.example.toml`](config.example.toml) to
`~/.config/tmux-notepad/config.toml` and point it at your notes:

```toml
[[sources]]
name      = "Notes"
path      = "~/notes"
recursive = true
exclude   = ["attachments"]
```

List as many sources as you like. A directory that is not reachable — a share
that is not mounted — is reported in the popup rather than being an error.

tmux-side options:

| Option | Default | Meaning |
| --- | --- | --- |
| `@notepad-key` | `n` | key after the prefix |
| `@notepad-width` | `80%` | popup width on first open |
| `@notepad-height` | `80%` | popup height on first open |
| `@notepad-command` | the plugin's own binary | path to the binary |

Setting `@notepad-command` also turns off the fetch-on-first-use above: name a
binary and that is the one that runs, always.

## Keys

| Key | Does |
| --- | --- |
| `enter` | paste into the pane and close |
| `tab` | paste and stay open |
| `x` | tick off / un-tick without pasting |
| `↑` `↓` / `j` `k` | move the selection |
| `/` | search titles and text |
| `f` | choose another note |
| `n` | new note |
| `a` | add an entry |
| `d` | delete the entry |
| `e` | open the note in `$EDITOR` |
| `r` | reload |
| `alt`+`←→↑↓` | move the window |
| `shift`+`←→↑↓` | resize the window |
| `+` `-` | bigger / smaller |
| `0` | back to the default size |
| `q` / `esc` | close |

The popup always opens at the size from the key binding, so the preview is
there from the start; a geometry you set by hand lasts for that session. A
window too narrow for two columns puts the preview below the list rather than
dropping it.

Window keys work in steps of eight columns and three rows. They close and
reopen the popup at the new geometry, because tmux cannot move a popup that is already open —
selection, search and scroll position come along, and the geometry is
remembered for next time. The travel range is whatever the popup does not
already fill, so a window at 80% has little room to move; shrink it with `-`
first. When a key changes nothing, the popup says which edge it is against.

## Editing from elsewhere

The open note is re-read when it changes on disk, so edits made in another
editor or a web UI show up while the popup is open. Writes are atomic and
refuse to overwrite a change made in the meantime — if the file moved under
it, the popup reloads and says so instead of clobbering your edit.

This is polling, not inotify, because notes often live on a network share
where inotify never sees another machine's writes.

## License

MIT — see [LICENSE](LICENSE).
