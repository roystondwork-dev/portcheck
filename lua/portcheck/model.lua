-- What the plugin shows, as plain data. Pure; tested in tests/nvim.
local M = {}

function M.holder_name(s)
  if s.container then
    local c = s.container
    return ("%s:%s"):format(c.runtime, c.name or c.image or c.id or "?")
  end
  if s.pid == nil then
    return "(hidden)"
  end
  return s.process and s.process.name or "?"
end

function M.started_by(s)
  local parts = {}
  for _, a in ipairs((s.process and s.process.ancestors) or {}) do
    table.insert(parts, ("%s (%d)"):format(a.name, a.pid))
  end
  return table.concat(parts, " ← ")
end

--- Listening sockets as rows, one per port, protocol and process, by port.
function M.rows(data)
  local rows, seen = {}, {}
  for _, s in ipairs(data.sockets or {}) do
    local name = M.holder_name(s)
    local id = ("%s/%d/%s"):format(s.proto, s.localPort, s.pid or name)
    if not seen[id] then
      seen[id] = true
      table.insert(rows, {
        port = s.localPort,
        proto = s.proto,
        name = name,
        pid = (not s.container) and s.pid or nil,
        user = s.process and s.process.user or nil,
        started_by = M.started_by(s),
        stop_hint = s.container and s.container.stopHint or nil,
      })
    end
  end
  table.sort(rows, function(a, b)
    if a.port ~= b.port then
      return a.port < b.port
    end
    return a.proto < b.proto
  end)
  return rows
end

function M.format_row(r)
  local who = r.pid and ("%s (pid %d)"):format(r.name, r.pid) or r.name
  local by = r.started_by ~= "" and ("  ← " .. vim.split(r.started_by, " ← ", { plain = true })[1]) or ""
  return ("%-6d %-4s %s%s"):format(r.port, r.proto, who, by)
end

--- A check result for one port: { status, summary, stoppable = {{pid, name}}, hint }.
function M.describe(port, data)
  local p
  for _, x in ipairs(data.ports or {}) do
    if x.port == port then
      p = x
    end
  end
  if not p or p.status == "free" then
    return { status = "free", summary = ("Port %d is free."):format(port), stoppable = {} }
  end
  if p.status == "denied" then
    return { status = "denied", summary = ("Port %d can't be used by your user."):format(port), stoppable = {} }
  end
  local stoppable, hint, names = {}, nil, {}
  for _, s in ipairs(p.holders or {}) do
    local name = M.holder_name(s)
    if s.container then
      hint = s.container.stopHint
    elseif s.pid == nil then
      hint = ("sudo portcheck kill %d"):format(port)
    elseif not vim.iter(stoppable):any(function(t)
      return t.pid == s.pid
    end) then
      table.insert(stoppable, { pid = s.pid, name = name })
    end
    local by = M.started_by(s)
    local who = (s.pid and not s.container) and ("%s (pid %d)"):format(name, s.pid) or name
    table.insert(names, by ~= "" and (who .. ", started by " .. by) or who)
  end
  return {
    status = "in-use",
    summary = ("Port %d is in use by %s."):format(port, table.concat(vim.fn.uniq(names), "; ")),
    stoppable = stoppable,
    hint = hint,
  }
end

function M.kill_summary(data)
  local ok, parts = #(data.results or {}) > 0, {}
  for _, r in ipairs(data.results or {}) do
    local who = ("%s (pid %d)"):format(r.name or "process", r.pid)
    if r.outcome == "failed" then
      ok = false
      table.insert(parts, ("%s: not stopped: %s"):format(who, r.error or "unknown error"))
    else
      table.insert(parts, ("%s: %s"):format(who, r.outcome))
    end
  end
  return ok, (#parts > 0 and table.concat(parts, "; ") or "nothing to stop")
end

return M
