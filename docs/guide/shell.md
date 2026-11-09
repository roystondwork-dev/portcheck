---
title: "Shell integration (init)"
description: "After a command fails with “address already in use”, your terminal says which process holds the port, who started it and how to free it: portcheck's shell hook for zsh, bash, fish and PowerShell."
---

# Shell integration: `init`

With the shell hook, a failed command that wanted a taken port gets an answer right under
its error, whatever tool it was:

```console
$ python3 -m http.server 8000
OSError: [Errno 48] Address already in use
portcheck: port 8000 is held by Python (pid 41852), started by zsh (900), in my-shop
portcheck: stop it: portcheck kill 8000  ·  or use a free port: portcheck run --port 8000 --next -- python3 -m http.server {port}
```

It prints nothing at all when a command succeeds, when it fails for another reason, or when
the ports it reads are free.

## Set up

Add one line to your shell's startup file, then open a new terminal:

=== "zsh"

    ```sh
    echo 'eval "$(portcheck init zsh)"' >> ~/.zshrc
    ```

=== "bash"

    ```sh
    echo 'eval "$(portcheck init bash)"' >> ~/.bashrc
    ```

=== "fish"

    ```sh
    echo 'portcheck init fish | source' >> ~/.config/fish/config.fish
    ```

=== "PowerShell"

    ```powershell
    Add-Content $PROFILE 'portcheck init powershell | Out-String | Invoke-Expression'
    ```

To remove it, delete that line.

## How it works

Your shell can't show portcheck a failed command's error message, only its command line
and exit status. So after a command fails, the hook runs
`portcheck explain --exit <status> -- '<command line>'`, which:

1. **Reads the ports the command line listens on.** First the explicit ones: `--port 3000`,
   `--port=3000`, `-p 3000`, `-p3000`, `PORT=3000 …`, `--listen :8080`, `--bind
   0.0.0.0:8000`, `0.0.0.0:8000` or `:8080` arguments, `runserver 8000`,
   `http.server 8000`, `php -S localhost:8000`, the host side of `docker run -p 8080:80`
   and `kubectl port-forward svc/web 8080:80`.
2. **Otherwise uses a known dev server's default port:**

    | Command | Port |
    |---|---|
    | `vite`, `vite dev` | 5173 (`vite preview`: 4173) |
    | `next dev`, `next start`, `nuxt dev`, `react-scripts start`, `rails server`, `npx serve` | 3000 |
    | `flask run` | 5000 |
    | `python manage.py runserver`, `uvicorn`, `gunicorn`, `fastapi dev`, `gatsby develop`, `php artisan serve`, `mkdocs serve`, `python -m http.server` | 8000 |
    | `ng serve` | 4200 |
    | `astro dev` | 4321 |
    | `jekyll serve` | 4000 |
    | `hugo server` | 1313 |

    Wrappers are looked through (`npx`, `pnpm exec`, `bunx`, `sudo`, `uv run`, `poetry run`,
    `bundle exec`, …), and so are package scripts: for `npm run dev`, `pnpm dev`, `yarn dev`
    or `bun run dev`, it reads the `dev` script from `./package.json` and applies the same
    rules to it. `npm start` without a recognisable script means port 3000.
3. **Checks those ports**, like [`check`](check.md), and speaks only when something holds
   one. Commands that connect to a port rather than listen on it (`ssh -p`, `psql -p`,
   `curl localhost:3000`, …) are ignored.

It takes a few milliseconds, gives up after one second, and never prints an error, so it
can't get in the way of your prompt.

The `or use a free port` suggestion appears when the port was on the command line, so it
can be swapped for `{port}` (see [`run`](run.md)).

!!! note "It's a heuristic"
    portcheck reads ports from the command line and known defaults, not from the error
    text. A command that fails for another reason while something happens to hold its port
    still gets the line; that is usually useful to know anyway. A server that reads its port
    from a config file isn't recognised.

## Skipped commands

The hook doesn't run for successful commands, Ctrl-C (exit status 130), suspended jobs
(148), commands that weren't found or aren't executable (127, 126), and portcheck itself.

## Shell notes

- **zsh:** uses `add-zsh-hook` (`preexec` and `precmd`), so it works next to Oh My Zsh,
  Powerlevel10k and starship.
- **bash:** adds a function to the front of `PROMPT_COMMAND` and keeps `$?` for the rest.
  It reads the command from history, so commands left out of history (`HISTCONTROL=ignorespace`
  with a leading space, or a repeat with `ignoredups`) are skipped.
- **fish:** uses the `fish_postexec` event.
- **PowerShell:** wraps your `prompt` function and reads `$LASTEXITCODE`, so it reacts to
  native programs (`npm`, `python`, …) that fail.

The snippet runs `portcheck` by name when it's on your `PATH`, so upgrades keep working, and
by its full path otherwise (for example when you use it through `npx`). Print it with
`portcheck init <shell>` to see exactly what it adds.

## Use `explain` yourself

```sh
portcheck explain -- 'npm run dev'            # who holds the port npm run dev wants
portcheck explain --json -- 'rails s -p 4000' # the ports read and their holders
```

See [JSON output](../json-schema.md#explain) for the `--json` format.
