-- Loaded at startup: the :Portcheck command and the :terminal watcher. The
-- rest of the plugin loads on first use.
if vim.g.loaded_portcheck then
  return
end
vim.g.loaded_portcheck = true

vim.api.nvim_create_user_command("Portcheck", function(opts)
  require("portcheck").command(opts)
end, {
  nargs = "*",
  desc = "See what's using a port and free it: list | check <port> | kill <port> | free [range]",
  complete = function(arglead, cmdline)
    return require("portcheck")._complete(arglead, cmdline)
  end,
})

vim.api.nvim_create_autocmd("TermOpen", {
  group = vim.api.nvim_create_augroup("portcheck", { clear = true }),
  callback = function(args)
    require("portcheck").watch_terminal(args.buf)
  end,
})
