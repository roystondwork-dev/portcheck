# Neovim

The portcheck plugin for Neovim (0.10 or later) lists your ports, stops what's holding
one, and notices "port already in use" in a `:terminal`.

```vim
:Portcheck               " pick a listening port: stop it, open it, copy its PID
:Portcheck check 3000    " who holds port 3000
:Portcheck kill 3000     " stop it, after confirming
:Portcheck free 3000-3999 " a free port, yanked to the unnamed register
```

When a dev server in a `:terminal` fails with "port already in use" (Node.js, Go,
Spring Boot, Python, Ruby, PHP, Vite, Next.js, Docker, …), the plugin says which
process holds the port and offers to stop it. Stopping keeps portcheck's
[safety guards](kill.md#safety).

## Install

[Install portcheck](../install.md), then add the plugin. With
[lazy.nvim](https://github.com/folke/lazy.nvim):

```lua
{
  "roystondwork-dev/portcheck",
  cmd = "Portcheck",
  event = "TermOpen",
  opts = {
    -- binary = "portcheck",  -- path to portcheck if it isn't on PATH
    -- terminal = "prompt",   -- "prompt", "notify", or false to not watch terminals
  },
}
```

Any other plugin manager works too: the plugin is the `lua/` and `plugin/` folders of
the repository. `:checkhealth portcheck` checks that portcheck is found.

Pickers use `vim.ui.select`, so they look like the rest of your setup (Telescope,
fzf-lua, snacks, dressing).
