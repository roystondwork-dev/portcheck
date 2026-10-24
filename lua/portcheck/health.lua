-- :checkhealth portcheck
local M = {}

function M.check()
  vim.health.start("portcheck")
  if vim.fn.has("nvim-0.10") == 0 then
    vim.health.error("Neovim 0.10 or later is required")
  end
  local cli = require("portcheck.cli")
  local err, data = cli.run({ "version" }, "version")
  if err then
    vim.health.error(err, { "Install portcheck: https://roystondwork-dev.github.io/portcheck/install/" })
  else
    vim.health.ok(("portcheck %s (%s)"):format(data.version, cli.binary))
  end
end

return M
