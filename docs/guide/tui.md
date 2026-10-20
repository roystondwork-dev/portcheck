# Interactive view: `tui`

![portcheck TUI](../assets/demo.gif)

Run `portcheck` with no arguments in a terminal, or `portcheck tui`, to get a live,
full-screen table of sockets. It starts with **listening sockets only** (press ++l++
for all), refreshes every 2 seconds (`--interval` changes this) and keeps your
selection in place when rows move.

When stdin or stdout is not a terminal (pipes, CI), bare `portcheck` prints help
instead, so scripts are never surprised by a full-screen UI; `portcheck tui` exits
with status 2.

## Keys

| Key | Action |
|---|---|
| ++up++ / ++down++ / ++j++ | move the selection |
| ++page-up++ / ++page-down++ | move one page |
| ++home++ ++g++ / ++end++ ++shift+g++ | first / last row |
| ++slash++ | filter: type to narrow, ++enter++ keeps the filter, ++esc++ clears it |
| ++esc++ | clear the filter |
| ++tab++ | cycle protocol: tcp+udp → tcp → udp |
| ++l++ | toggle listening-only / all sockets |
| ++s++ / ++shift+s++ | next sort column (port, proto, state, pid, process, user) / reverse order |
| ++r++ | refresh now |
| ++c++ | copy the selected PID to the clipboard (OSC 52) |
| ++x++ ++k++ ++delete++ | stop the selected process gracefully (asks first) |
| ++shift+k++ | force-kill the selected process (asks first) |
| ++question++ | help |
| ++q++ / ++ctrl+c++ | quit |

The filter matches port, protocol, local and remote address, state, PID, process
name and user, case-insensitively.

## Colors

- **Green** rows are TCP listeners.
- **Dimmed** rows have an owner hidden from your user. Stopping one shows the
  privilege hint; see [Permissions by OS](../permissions.md).

Colors adapt to your terminal and are disabled with `NO_COLOR=1` or `--no-color`.

## Stopping processes

++x++ asks *"Stop node (pid 42379) listening on 3000? [y/N]"*. On ++y++, portcheck uses the
same graceful-then-force procedure and [safety guards](kill.md#safety) as `portcheck kill`
(with a 3 second grace period), shows the result in the status line, and refreshes.

!!! tip "Copying PIDs over SSH"
    ++c++ uses the OSC 52 escape sequence, which most modern terminals support,
    including over SSH and inside tmux (with `set -g set-clipboard on`).
