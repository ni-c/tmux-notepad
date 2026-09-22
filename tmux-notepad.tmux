#!/usr/bin/env bash
# tpm entry point: binds the key that opens the notepad popup.
#
# display-popup is used directly rather than through run-shell: run-shell -b
# has no client and so cannot show a popup at all, and without -b it blocks
# the client for as long as the popup is open. The pane to paste into is
# resolved by tmux-notepad itself (tmux display-message -p '#{pane_id}' inside
# a popup reports the pane the popup sits over), so no format expansion is
# needed here — which is just as well, since display-popup does not expand
# formats in the command it runs.

set -eu

option() {
  local value
  value="$(tmux show-option -gqv "$1")"
  [ -n "$value" ] || value="$2"
  printf '%s' "$value"
}

KEY="$(option '@notepad-key' 'n')"
WIDTH="$(option '@notepad-width' '80%')"
HEIGHT="$(option '@notepad-height' '80%')"
BIN="$(option '@notepad-command' 'tmux-notepad')"

tmux bind-key "$KEY" display-popup -E -w "$WIDTH" -h "$HEIGHT" -T ' notepad ' "$BIN"
