-- Recognises "port already in use" errors in terminal output. The common
-- messages of extensions/vscode/src/matcher.ts, as Lua patterns.
local M = {}

local PATTERNS = {
  { "Node.js", "EADDRINUSE.-:(%d+)" },
  { "Go", "listen [tu][cd]p%d? %S-:(%d+): bind: [Aa]ddress already in use" },
  { "Go", "listen [tu][cd]p%d? %S-:(%d+): bind: Only one usage of each socket address" },
  { "Docker", "Bind for %S-:(%d+) failed: port is already allocated" },
  { "Ruby", 'Address already in use %- bind%(2%) for "?[^"%s]*"? port (%d+)' },
  { "Spring Boot", "Port (%d+) was already in use" },
  { ".NET", "Failed to bind to address %S-:(%d+)" },
  { "Python", "error while attempting to bind on address %('[^']*', (%d+)%)" },
  { "Flask", "Port (%d+) is in use by another program" },
  { "PHP", "Failed to listen on %S-:(%d+) %(reason: Address already in use%)" },
  { "PostgreSQL", "another postmaster already running on port (%d+)" },
  { "Create React App", "Something is already running on port (%d+)" },
  { "Bun", "Failed to start server%. Is port (%d+) in use%?" },
  { "dev server", "[Pp]ort (%d+) is already in use" },
  { "dev server", "[Pp]ort (%d+) is in use, trying" },
  { "unknown", "[Aa]ddress already in use%S*%s+%S-:(%d+)" },
}

-- Terminal escape sequences (colours, cursor moves, OSC titles).
local function strip(s)
  return (s:gsub("\27%[[0-?]*[ -/]*[@-~]", ""):gsub("\27%][^\7\27]*\7", ""):gsub("\27%][^\7\27]*\27\\", ""))
end

--- The port and source of a "port in use" error on this line, or nil.
---@return integer|nil port, string|nil source
function M.match(line)
  line = strip(line)
  for _, p in ipairs(PATTERNS) do
    local port = tonumber(line:match(p[2]))
    if port and port >= 1 and port <= 65535 then
      return port, p[1]
    end
  end
  return nil
end

return M
