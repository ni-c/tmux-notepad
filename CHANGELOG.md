# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

<!-- The release workflow extracts the section of the version being tagged with
     awk, matching "## [x.y.z]". Keep that heading shape exactly. -->

## [Unreleased]

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
