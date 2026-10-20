# Exit codes

portcheck's exit codes are stable and safe to use in scripts.

| Code | Name | Meaning | Examples |
|---|---|---|---|
| `0` | OK | Success, and the answer is "yes" | all checked ports are free; free ports found; processes stopped; `scan` found an open port; `wait`: every target ready |
| `1` | Negative | The command worked, but the answer is "no" | `check`: a port is in use or denied · `free`: not enough free ports · `kill`: nothing listening, PID not found, user declined, protected process refused, port belongs to a container · `scan`: no open ports · `wait`: timed out |
| `2` | Usage | Invalid arguments or flags | unknown flag, bad port spec, `kill` without `--yes` in a non-interactive shell, `scan` target too large or malformed, `tui` without a terminal |
| `3` | Runtime | Unexpected error | the OS query failed; the process survived `kill`; a `scan` hostname did not resolve |
| `4` | Permission | More privileges needed | owner of a port is hidden; not allowed to signal the process |
| `130` | Interrupted | Cancelled with Ctrl-C | `scan` and `wait` still print the results gathered so far |

`portcheck run` is the exception: once the command starts, portcheck exits with
**the command's** status (`128 + signal` if it was killed by a signal). If the
command can't be started, `run` uses the codes above, plus `126` (not executable)
and `127` (not found) as in a shell. See [`run`](guide/run.md#exit-status).

Error messages go to **stderr** and results go to **stdout**, so
`portcheck … --json | jq` keeps working even when warnings or hints are printed.

```sh
if portcheck check 5432 >/dev/null; then
  echo "5432 is free"
else
  case $? in
    1) echo "5432 is taken" ;;
    *) echo "could not check 5432" >&2 ;;
  esac
fi
```
