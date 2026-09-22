# Security

## Reporting a vulnerability

Report it privately through
[GitHub Private Vulnerability Reporting](https://github.com/ni-c/tmux-notepad/security/advisories/new).
Please do not open a public issue for anything exploitable.

Expect a first reply within a week. Fixes go to the newest release and to `main`;
older releases are not patched.

## What this program does

Worth knowing before you audit it, because most of it is narrower than a tmux
plugin sounds:

- It reads and writes the Markdown files you point it at in `config.toml`, and
  nothing else. Ticking an entry off rewrites one heading line in place, through
  a temporary file, and refuses the write if the file changed underneath it.
- It runs `tmux` — to find the pane it was opened over, to paste, and to reopen
  its own popup at a new size. Nothing else is executed except the `$EDITOR` you
  configured, when you press `e`.
- **It never presses Enter.** Text is delivered as a bracketed paste and left on
  the input line. That is the property that makes it safe to point at a coding
  agent, and it is covered by tests; treat a change that submits on your behalf
  as a security bug, not a feature request.
- At runtime it makes no network connections at all.

Your notes are not a trust boundary — they are your own files, shown to you.
The program does not interpret them as commands, and the pasted text is never
run by it.

## Installing: what you are trusting

The two install paths have different trust anchors, and it is worth being plain
about which is which.

**The one-liner** is the stronger of the two:

```sh
curl -fsSL https://github.com/ni-c/tmux-notepad/releases/latest/download/install.sh | bash
```

That installer carries the SHA-256 of every release archive inside it. It checks
the archive it downloads against the one for your platform and refuses to install
on a mismatch. You can read the script before running it, and verify it came out
of this repository's release workflow:

```sh
curl -fsSLO https://github.com/ni-c/tmux-notepad/releases/latest/download/install.sh
gh attestation verify install.sh --repo ni-c/tmux-notepad
```

Every release also ships `tmux-notepad.intoto.jsonl`, so the same check works
without GitHub's attestation API:

```sh
gh attestation verify install.sh --bundle tmux-notepad.intoto.jsonl \
  --repo ni-c/tmux-notepad
```

**The tpm path** is one link longer. `set -g @plugin 'ni-c/tmux-notepad'` gives
you a git checkout with no binary in it; the first time you press the key, the
popup fetches that same `install.sh` over HTTPS and runs it. The archive is still
checked against the checksum baked into the downloaded installer — but the
installer itself arrives on the strength of TLS to github.com, not a signature
you pinned. If that is not good enough for you, install the binary yourself with
the one-liner or `go install`, set `@notepad-command` to it, and the plugin will
never fetch anything.

Neither path needs root, and neither writes outside `~/.local/bin`,
`~/.config/tmux-notepad/` and a marked block in `~/.tmux.conf`.

## Supported versions

| Version | Supported |
| --- | --- |
| newest release | yes |
| anything older | no |
