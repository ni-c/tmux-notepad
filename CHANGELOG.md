# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

<!-- The release workflow extracts the section of the version being tagged with
     awk, matching "## [x.y.z]". Keep that heading shape exactly. -->

## [Unreleased]

### Fixed

- An entry whose title ended in a hash was renamed the first time it was ticked
  off: `# C#` parsed as `C`, because any trailing hash was trimmed as a closing
  sequence. CommonMark only treats one as decoration when a space sets it off,
  which is now what happens. Headings that ATX cannot express at all — `# # #`,
  `# 0 # #`, `# ✓#` — are no longer entries, because rendering them back would
  drop them out of the note. Found by `FuzzParse`.
- `make install` failed on macOS: `install -D` is a GNU extension.

### Security

- Updated three dependencies with known vulnerabilities. Two were reachable from
  this code: [GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970), an infinite
  loop on invalid input in `golang.org/x/text`, and
  [GO-2026-5320](https://pkg.go.dev/vuln/GO-2026-5320), an XSS in the HTML
  renderer of `github.com/yuin/goldmark` reached through glamour — which renders
  to ANSI rather than to a browser, so it had nowhere to land.
  [GO-2026-5942](https://pkg.go.dev/vuln/GO-2026-5942) in
  `golang.org/x/net/dns/dnsmessage` was not reachable, since this program opens
  no network connections; it was updated anyway.
- CI now runs `govulncheck` on every pull request and weekly.

## [1.0.0] - 2026-09-22

### Added

- Popup listing the entries of a Markdown note, with a rendered preview. A
  window too narrow for two columns stacks the preview below the list rather
  than dropping it.
- Paste an entry into the pane the popup was opened from, as a bracketed paste
  and without submitting it.
- Tick off pasted entries in the note itself as `# ✓ Title`.
- Search across titles and bodies, note picker across several configured
  source directories, and creating a note with a name suggested from the
  target pane's project.
- Add and delete entries, and open the note in `$EDITOR`.
- Move and resize the popup with the keyboard, in steps of eight columns and
  three rows. Geometry and selection survive the reopen and are remembered,
  the popup stays clear of the status line, and a key that changes nothing
  says which edge the window is against.
- Re-read the open note when it changes on disk, with atomic writes that
  refuse to overwrite a concurrent change.
- One-command install: `install.sh`, shipped with every release with the
  checksum of each platform archive baked in. With tpm, the binary is fetched
  on first use inside the popup, so tmux never waits on the network.

[Unreleased]: https://github.com/ni-c/tmux-notepad/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/ni-c/tmux-notepad/releases/tag/v1.0.0
