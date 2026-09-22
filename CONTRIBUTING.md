# Contributing

## Before a pull request

```sh
make check
```

That is `gofmt`, `go vet` and the tests. CI runs the same thing on Linux and
macOS, plus `golangci-lint`, short fuzzing runs, a cross-compile of all four
release targets and an end-to-end run of the installer.

## The tests need tmux

Most of what is worth testing here is what tmux does with a popup, so those
tests start a throwaway tmux server on their own socket and point `$TMUX` at it.
Your own session is never touched — `TestMain` unsets `TMUX` first.

Without tmux installed, those tests skip themselves so that `go test ./...`
still works. That is a trap on a build machine: a run of nothing but skips is
green and proves nothing. So CI sets

```sh
TMUX_NOTEPAD_REQUIRE_TMUX=1 go test ./...
```

which turns every one of those skips into a failure. If you are changing
anything under `internal/tmuxio` or the key handling in `internal/ui`, run it
that way too. See `internal/testenv`.

## Releasing

1. Move the `## [Unreleased]` section of `CHANGELOG.md` to `## [x.y.z] - <date>`,
   add an empty `[Unreleased]` above it, and add the two link definitions at the
   bottom. The release workflow extracts the notes with `awk` by matching
   `## [x.y.z]` exactly, and fails if it finds nothing.
2. Set `VERSION` to `x.y.z`. This is not the build version — `make build` takes
   that from `git describe`. It is what a **tpm checkout** reads to know which
   release to fetch, because tpm clones the default branch rather than the tag.
   The release workflow refuses a tag that disagrees with this file.
3. Commit both, then `git tag -s vx.y.z` and push the tag.

The workflow builds the four archives, bakes their checksums into `install.sh`,
attests the lot and creates the release. Never move a tag afterwards: the
provenance and `gh release create --verify-tag` are tied to the tag's commit. A
mistake gets a new patch release.

## Style

- English everywhere — code, comments, commit messages, pull requests.
- Comments explain *why*, not *what*. The existing ones are the standard.
- Commit messages say what the change does in the imperative, on one line, with
  a paragraph below when the reason is not obvious from the diff.
