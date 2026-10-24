-- A thin client for the portcheck CLI's JSON output, like the editor
-- extensions: no port logic here, only `portcheck … --json`.
local M = {}

M.binary = "portcheck"

--- Runs portcheck with --json. `ok` lists exit codes that are answers (1 =
--- "in use" for check). Calls cb(err, data) on the main loop, or returns
--- (err, data) when cb is nil.
---@param args string[]
---@param kind string
---@param ok integer[]|nil
---@param cb fun(err: string|nil, data: table|nil)|nil
function M.run(args, kind, ok, cb)
  ok = ok or { 0 }
  local cmd = { M.binary }
  vim.list_extend(cmd, args)
  table.insert(cmd, "--json")

  local function finish(res)
    local out = vim.trim(res.stdout or "")
    if not vim.tbl_contains(ok, res.code) or out == "" then
      local line = vim.trim(vim.split(vim.trim(res.stderr or ""), "\n")[1] or "")
      line = line:gsub("^portcheck: ", "")
      return (line ~= "" and line or ("portcheck exited with status " .. res.code)), nil
    end
    local decoded, env = pcall(vim.json.decode, out)
    if not decoded or type(env) ~= "table" then
      return ('portcheck printed invalid JSON for "%s"'):format(kind), nil
    end
    if env.schemaVersion ~= 1 or env.kind ~= kind then
      return ("unexpected output from portcheck (schema %s, kind %s); update portcheck"):format(
        tostring(env.schemaVersion),
        tostring(env.kind)
      ),
        nil
    end
    return nil, env.data
  end

  local started, proc = pcall(vim.system, cmd, { text = true }, cb and function(res)
    vim.schedule(function()
      cb(finish(res))
    end)
  end or nil)
  if not started then
    local err = "portcheck isn't installed (or not on PATH): " .. tostring(proc)
    if cb then
      vim.schedule(function()
        cb(err, nil)
      end)
      return
    end
    return err, nil
  end
  if not cb then
    return finish(proc:wait())
  end
end

function M.list(cb)
  return M.run({ "list", "--listening" }, "list", { 0, 4 }, cb)
end

function M.check(port, cb)
  return M.run({ "check", tostring(port) }, "check", { 0, 1 }, cb)
end

function M.free(range, cb)
  return M.run(range and { "free", range } or { "free" }, "free", { 0, 1 }, cb)
end

function M.kill(pid, cb)
  return M.run({ "kill", "--pid", tostring(pid), "--yes" }, "kill", { 0, 1, 3, 4 }, cb)
end

return M
