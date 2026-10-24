-- portcheck for Neovim: see what's using a port and free it safely.
--   :Portcheck              pick a listening port: stop it, open it, copy its PID
--   :Portcheck check 3000   who holds port 3000
--   :Portcheck kill 3000    stop it, after confirming
--   :Portcheck free [range] a free port, yanked to the unnamed register
-- When a :terminal prints "port already in use", the plugin says who holds it.
local cli = require("portcheck.cli")
local model = require("portcheck.model")
local matcher = require("portcheck.matcher")

local M = {}

M.config = {
  --- Path to portcheck (default: on PATH).
  binary = "portcheck",
  --- On "port in use" in a :terminal: "prompt" (offer to stop it), "notify", or false.
  terminal = "prompt",
}

local function notify(msg, level)
  vim.notify(msg, level or vim.log.levels.INFO, { title = "portcheck" })
end

local function stop(pid, name, after)
  vim.ui.select({ "Stop", "Cancel" }, { prompt = ("Stop %s (pid %d)?"):format(name, pid) }, function(choice)
    if choice ~= "Stop" then
      return
    end
    cli.kill(pid, function(err, data)
      if err then
        return notify(err, vim.log.levels.ERROR)
      end
      local ok, text = model.kill_summary(data)
      notify(text, ok and vim.log.levels.INFO or vim.log.levels.WARN)
      if after then
        after(ok)
      end
    end)
  end)
end

--- Offers the fixes for a busy port: stop each process, or copy a command.
local function offer(port, d)
  local choices = {}
  for _, t in ipairs(d.stoppable) do
    table.insert(choices, { label = ("Stop %s (pid %d)"):format(t.name, t.pid), stop = t })
  end
  if d.hint then
    table.insert(choices, { label = "Copy: " .. d.hint, copy = d.hint })
  end
  if #choices == 0 then
    return notify(d.summary)
  end
  table.insert(choices, { label = "Nothing" })
  vim.ui.select(choices, {
    prompt = d.summary,
    format_item = function(c)
      return c.label
    end,
  }, function(c)
    if c and c.stop then
      stop(c.stop.pid, c.stop.name)
    elseif c and c.copy then
      vim.fn.setreg('"', c.copy)
      notify("Copied: " .. c.copy)
    end
  end)
end

function M.check(port, then_offer)
  cli.check(port, function(err, data)
    if err then
      return notify(err, vim.log.levels.ERROR)
    end
    local d = model.describe(port, data)
    if then_offer and d.status == "in-use" then
      offer(port, d)
    else
      notify(d.summary)
    end
  end)
end

function M.pick()
  cli.list(function(err, data)
    if err then
      return notify(err, vim.log.levels.ERROR)
    end
    local rows = model.rows(data)
    if #rows == 0 then
      return notify("Nothing is listening.")
    end
    vim.ui.select(rows, { prompt = "Listening ports", format_item = model.format_row }, function(r)
      if not r then
        return
      end
      local actions = {}
      if r.pid then
        table.insert(actions, "Stop " .. r.name)
      end
      if r.stop_hint then
        table.insert(actions, "Copy stop command")
      end
      vim.list_extend(actions, { "Open http://localhost:" .. r.port, "Copy PID" })
      vim.ui.select(actions, { prompt = model.format_row(r) }, function(a)
        if not a then
          return
        elseif a:match("^Stop") then
          stop(r.pid, r.name)
        elseif a == "Copy stop command" then
          vim.fn.setreg('"', r.stop_hint)
        elseif a:match("^Open") then
          vim.ui.open("http://localhost:" .. r.port .. "/")
        elseif r.pid then
          vim.fn.setreg('"', tostring(r.pid))
          notify("Copied PID " .. r.pid)
        end
      end)
    end)
  end)
end

function M.free(range)
  cli.free(range, function(err, data)
    if err then
      return notify(err, vim.log.levels.ERROR)
    end
    local port = data.ports and data.ports[1]
    if not port then
      return notify("No free port" .. (range and (" in " .. range) or ""), vim.log.levels.WARN)
    end
    vim.fn.setreg('"', tostring(port))
    notify(("Port %d is free (yanked)"):format(port))
  end)
end

--- Watches a terminal buffer for "port in use" errors (each port once).
function M.watch_terminal(buf)
  if not M.config.terminal then
    return
  end
  local seen = {}
  vim.api.nvim_buf_attach(buf, false, {
    on_lines = function(_, b, _, first, _, last)
      for _, line in ipairs(vim.api.nvim_buf_get_lines(b, first, last, false)) do
        local port = matcher.match(line)
        if port and not seen[port] then
          seen[port] = true
          vim.schedule(function()
            M.check(port, M.config.terminal == "prompt")
          end)
        end
      end
    end,
  })
end

local function complete(arglead, cmdline)
  if #vim.split(cmdline, "%s+", { trimempty = true }) <= 2 and not cmdline:match("%s%S+%s") then
    return vim.tbl_filter(function(c)
      return c:find(arglead, 1, true) == 1
    end, { "check", "kill", "free", "list" })
  end
  return {}
end

function M.command(opts)
  local sub, arg = opts.fargs[1], opts.fargs[2]
  local port = tonumber(arg)
  if sub == nil or sub == "list" then
    return M.pick()
  elseif (sub == "check" or sub == "kill") and port then
    return M.check(port, sub == "kill")
  elseif sub == "free" then
    return M.free(arg)
  end
  notify("usage: :Portcheck [list] | check <port> | kill <port> | free [range]", vim.log.levels.WARN)
end

function M.setup(opts)
  M.config = vim.tbl_extend("force", M.config, opts or {})
  cli.binary = M.config.binary
end

M._complete = complete

return M
