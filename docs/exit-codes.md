# Exit codes

portcheck's exit codes are stable and safe to use in scripts.

| Code | Name | Meaning | Examples |
|---|---|---|---|
| `0` | OK | Success, and the answer is "yes" | all checked ports are free; free ports found; processes stopped; `scan` found an open port; `wait`: every target ready |
| `1` | Negative | The command worked, but the answer is "no" | `check`: a port is in use or denied · `free`: not enough free ports · `kill`: nothing listening, user declined, protected process refused · `scan`: no open ports · `wait`: timed out |
| `2` | Usage | Invalid arguments or flags | unknown flag, bad port spec, `kill` without `--yes` in a non-interactive shell, `scan` target too large or malformed |
| `3` | Runtime | Unexpected error | the OS query failed; the process survived `kill`; a `scan` hostname did not resolve |
| `4` | Permission | More privileges needed | owner of a port is hidden; not allowed to signal the process |
| `130` | Interrupted | Cancelled with Ctrl-C | `scan` still prints the results gathered so far |

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
