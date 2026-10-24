-- Tests for the Neovim plugin, in headless Neovim:
--   nvim --headless -u NONE --cmd "set rtp^=." -l tests/nvim/run.lua
-- PORTCHECK_BIN=<path> also runs the commands and the :terminal watcher
-- against the real portcheck.
vim.opt.rtp:prepend(".")
vim.cmd("runtime plugin/portcheck.lua")

local failures, passed = 0, 0
local function test(name, fn)
  local ok, err = pcall(fn)
  if ok then
    passed = passed + 1
  else
    failures = failures + 1
    io.stderr:write(("FAIL %s\n  %s\n"):format(name, err))
  end
end
local function eq(want, got, what)
  if not vim.deep_equal(want, got) then
    error(("%s: want %s, got %s"):format(what or "value", vim.inspect(want), vim.inspect(got)), 2)
  end
end

local matcher = require("portcheck.matcher")
local model = require("portcheck.model")
local cli = require("portcheck.cli")
local portcheck = require("portcheck")

test("matcher recognises real errors", function()
  for _, c in ipairs({
    { "Error: listen EADDRINUSE: address already in use :::3000", 3000, "Node.js" },
    { "Error: listen EADDRINUSE: address already in use 127.0.0.1:8080", 8080, "Node.js" },
    { "Error: listen EADDRINUSE :::3000", 3000, "Node.js" },
    { "listen tcp :8080: bind: address already in use", 8080, "Go" },
    { "listen tcp4 0.0.0.0:5432: bind: address already in use", 5432, "Go" },
    { "listen tcp :8080: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted.", 8080, "Go" },
    { "Bind for 0.0.0.0:8080 failed: port is already allocated", 8080, "Docker" },
    { 'Address already in use - bind(2) for "127.0.0.1" port 3000 (Errno::EADDRINUSE)', 3000, "Ruby" },
    { "Web server failed to start. Port 8080 was already in use.", 8080, "Spring Boot" },
    { "System.IO.IOException: Failed to bind to address http://127.0.0.1:5000: address already in use.", 5000, ".NET" },
    { "ERROR:    [Errno 48] error while attempting to bind on address ('127.0.0.1', 8000): address already in use", 8000, "Python" },
    { "Port 5000 is in use by another program.", 5000, "Flask" },
    { "[Tue Oct 20 12:00:00 2026] Failed to listen on localhost:8000 (reason: Address already in use)", 8000, "PHP" },
    { "HINT:  Is another postmaster already running on port 5432?", 5432, "PostgreSQL" },
    { "? Something is already running on port 3000.", 3000, "Create React App" },
    { "error: Failed to start server. Is port 3000 in use?", 3000, "Bun" },
    { "Error: Port 5173 is already in use", 5173, "dev server" },
    { " ⚠ Port 3000 is in use, trying 3001 instead.", 3000, "dev server" },
    { "socket.error: address already in use 0.0.0.0:9229", 9229, "unknown" },
    { "\27[31mError: listen EADDRINUSE: address already in use :::4000\27[0m", 4000, "Node.js" },
  }) do
    local port, source = matcher.match(c[1])
    eq({ c[2], c[3] }, { port, source }, c[1])
  end
end)

test("matcher ignores other lines", function()
  for _, line in ipairs({ "Server listening on http://localhost:3000", "Port 3000 is free.", "Error: listen EADDRINUSE", "port 70000 is already in use", "" }) do
    eq(nil, (matcher.match(line)), line)
  end
end)

local node = {
  proto = "tcp", localAddress = "127.0.0.1", localPort = 3000, pid = 42,
  process = { pid = 42, name = "node", user = "alice", ancestors = { { pid = 7, name = "npm" }, { pid = 3, name = "zsh" } } },
}
local web = { proto = "tcp", localAddress = "0.0.0.0", localPort = 8080, pid = 9, container = { name = "web", runtime = "docker", stopHint = "docker stop web" } }
local hidden = { proto = "tcp", localAddress = "0.0.0.0", localPort = 22 }

test("rows are unique, sorted and describe the holder", function()
  local rows = model.rows({ sockets = { web, node, node, hidden } })
  eq({ 22, 3000, 8080 }, vim.tbl_map(function(r) return r.port end, rows), "ports")
  eq({ "(hidden)", "node", "docker:web" }, vim.tbl_map(function(r) return r.name end, rows), "names")
  eq(42, rows[2].pid, "pid")
  eq(nil, rows[3].pid, "container pid")
  eq("docker stop web", rows[3].stop_hint, "hint")
  eq("3000   tcp  node (pid 42)  ← npm (7)", model.format_row(rows[2]), "format")
  eq("22     tcp  (hidden)", model.format_row(rows[1]), "format hidden")
  eq("podman:redis", model.holder_name({ container = { runtime = "podman", image = "redis" } }), "image")
  eq("docker:?", model.holder_name({ container = { runtime = "docker" } }), "anonymous")
  eq("?", model.holder_name({ pid = 1 }), "nameless")
end)

test("describe a check", function()
  local busy = { ports = { { port = 3000, status = "in-use", holders = { node, node } } } }
  local d = model.describe(3000, busy)
  eq("Port 3000 is in use by node (pid 42), started by npm (7) ← zsh (3).", d.summary, "summary")
  eq({ { pid = 42, name = "node" } }, d.stoppable, "stoppable")
  eq("free", model.describe(3000, { ports = {} }).status, "free")
  eq("Port 80 can't be used by your user.", model.describe(80, { ports = { { port = 80, status = "denied" } } }).summary, "denied")
  local other = model.describe(8080, { ports = { { port = 8080, status = "in-use", holders = { web } } } })
  eq({ {}, "docker stop web" }, { other.stoppable, other.hint }, "container")
  eq("sudo portcheck kill 22", model.describe(22, { ports = { { port = 22, status = "in-use", holders = { hidden } } } }).hint, "hidden")
end)

test("kill summaries", function()
  eq({ true, "node (pid 42): terminated" }, { model.kill_summary({ results = { { pid = 42, name = "node", outcome = "terminated" } } }) })
  eq({ false, "process (pid 1): not stopped: protected" }, { model.kill_summary({ results = { { pid = 1, outcome = "failed", error = "protected" } } }) })
  eq({ false, "nothing to stop" }, { model.kill_summary({ results = {} }) })
end)

test("cli reports errors clearly", function()
  local dir = vim.fn.tempname()
  vim.fn.mkdir(dir, "p")
  local function fake(script)
    local f = dir .. "/pc" .. math.random(1e6)
    vim.fn.writefile({ "#!/bin/sh", script }, f)
    vim.fn.setfperm(f, "rwxr-xr-x")
    return f
  end
  local old = cli.binary
  cli.binary = fake([[echo '{"schemaVersion":1,"kind":"check","data":{"ports":[]}}'; exit 1]])
  eq({ nil, { ports = {} } }, { cli.check(3000) }, "answer code")
  cli.binary = fake([[echo; echo 'portcheck: unknown flag --x' >&2; exit 2]])
  eq("unknown flag --x", (cli.list()), "stderr")
  cli.binary = fake("exit 3")
  eq("portcheck exited with status 3", (cli.list()), "status")
  cli.binary = fake("echo '{oops'")
  eq('portcheck printed invalid JSON for "list"', (cli.list()), "json")
  cli.binary = fake([[echo '{"schemaVersion":2,"kind":"list","data":{}}']])
  eq("unexpected output from portcheck (schema 2, kind list); update portcheck", (cli.list()), "schema")
  cli.binary = dir .. "/missing"
  assert((cli.list()):match("^portcheck isn't installed"), "missing binary")
  cli.binary = old
end)

test("the command parses its arguments", function()
  eq({ "check", "kill", "free", "list" }, portcheck._complete("", "Portcheck "), "complete")
  eq({ "free" }, portcheck._complete("f", "Portcheck f"), "complete prefix")
  eq({}, portcheck._complete("", "Portcheck check "), "no completion after subcommand")
  local warned
  local orig = vim.notify
  vim.notify = function(msg) warned = msg end
  portcheck.command({ fargs = { "bogus" } })
  vim.notify = orig
  assert(warned and warned:match("^usage:"), "usage")
end)

local bin = os.getenv("PORTCHECK_BIN")
if bin and bin ~= "" then
  portcheck.setup({ binary = bin })

  -- Holds a port from this Neovim with a uv TCP server.
  local server = vim.uv.new_tcp()
  server:bind("127.0.0.1", 0)
  server:listen(8, function() end)
  local port = server:getsockname().port
  local me = vim.fn.getpid()

  local function wait(what, fn)
    if not vim.wait(15000, fn, 50) then
      error("timed out waiting for " .. what, 2)
    end
  end

  test("real: :Portcheck check names this Neovim", function()
    local msgs = {}
    local orig = vim.notify
    vim.notify = function(m) table.insert(msgs, m) end
    vim.cmd("Portcheck check " .. port)
    wait("a message", function() return #msgs > 0 end)
    vim.notify = orig
    assert(msgs[1]:match(("^Port %d is in use by nvim %%(pid %d%%)"):format(port, me)), msgs[1])
  end)

  test("real: :Portcheck free yanks a free port", function()
    vim.fn.setreg('"', "")
    vim.cmd("Portcheck free 40000-40100")
    wait("the register", function() return tonumber(vim.fn.getreg('"')) ~= nil end)
  end)

  test("real: :Portcheck lists this port", function()
    local items
    local orig = vim.ui.select
    vim.ui.select = function(list, opts, cb) items = vim.tbl_map(opts.format_item or tostring, list) end
    vim.cmd("Portcheck")
    wait("the picker", function() return items ~= nil end)
    vim.ui.select = orig
    assert(vim.iter(items):any(function(i) return i:match("^" .. port .. " ") and i:match("pid " .. me) end), vim.inspect(items))
  end)

  test("real: a :terminal error offers to stop the holder", function()
    local prompt, choices
    local orig = vim.ui.select
    vim.ui.select = function(list, opts, cb)
      prompt, choices = opts.prompt, vim.tbl_map(opts.format_item or tostring, list)
    end
    vim.cmd("enew")
    vim.fn.jobstart({ "sh", "-c", ("printf 'Error: listen EADDRINUSE: address already in use :::%d\\n'; sleep 1"):format(port) }, { term = true })
    wait("the offer", function() return prompt ~= nil end)
    vim.ui.select = orig
    assert(prompt:match(("^Port %d is in use by nvim"):format(port)), prompt)
    eq(("Stop nvim (pid %d)"):format(me), choices[1], "first choice")
  end)

  server:close()
end

print(("%d passed, %d failed"):format(passed, failures))
os.exit(failures == 0 and 0 or 1)
