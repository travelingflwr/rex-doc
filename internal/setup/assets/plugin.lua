-- rdxt for Rex: open and create Markdown and Dok documents from the palette.
-- Installed by `rdxt setup`, which loads it from ~/.config/rex/init.lua.
-- `rdxt setup --undo` removes it.

local M = {}

local SHELL = "com.superlogical.terminal.shell"
local TERMINAL = "com.superlogical.terminal"
local FLOAT = { 0.1, 0.06, 0.8, 0.88 }

local function file_path(url)
  if type(url) ~= "string" then return nil end
  local path = url:match("^file://[^/]*(/.*)$")
  if path == nil then return nil end
  return (path:gsub("%%(%x%x)", function(hex) return string.char(tonumber(hex, 16)) end))
end

-- The folder of the program in front of the focused pane, or the shell's.
local function cwd(ctx)
  if ctx.block_id ~= nil then
    local info = rex.block.call(TERMINAL, "process", { block_id = ctx.block_id })
    if type(info) == "table" and type(info.foreground) == "table"
      and type(info.foreground.cwd) == "string" and info.foreground.cwd ~= "" then
      return info.foreground.cwd
    end
    local pwd = rex.block.call(TERMINAL, "pwd", { block_id = ctx.block_id })
    if type(pwd) == "table" then
      local p = file_path(pwd.pwd)
      if p ~= nil then return p end
    end
  end
  return os.getenv("HOME")
end

local function dirname(path) return path:match("^(.*)/[^/]*$") or "/" end
local function basename(path) return path:match("([^/]+)$") or path end

function M.setup(opts)
  opts = opts or {}
  local bin = opts.bin or "rdxt"

  -- show opens a document where `rdxt setup --open` said: a floating
  -- window, or a pane beside the focused one.
  local function show(ctx, path, extra)
    local command = { bin }
    for _, a in ipairs(extra or {}) do command[#command + 1] = a end
    command[#command + 1] = path
    local layout = { block = { flavor = SHELL, label = basename(path),
      options = { command = command, cwd = dirname(path) } } }
    if opts.open == "pane" then
      local _, err = rex.session.new_split{
        anchor_id = ctx.block_id, direction = "horizontal", side = "after",
        focus = true, layout = layout,
      }
      if err ~= nil then error("rdxt: couldn't open a pane: " .. tostring(err), 0) end
      return
    end
    local _, err = rex.session.new_layer{ bounds = FLOAT, focus = true, layout = layout }
    if err ~= nil then error("rdxt: couldn't open a floating window: " .. tostring(err), 0) end
  end

  local function remote(ctx)
    if ctx.server ~= nil then
      error("rdxt: documents on remote hosts aren't supported from the palette yet. Run rdxt there.", 0)
    end
  end

  rex.action{
    name = "rdxt_open",
    title = "rdxt: Open Document…",
    description = "Open a Markdown or Dok document, recent or in this folder, in rdxt.",
    category = "rdxt",
    keywords = "rdxt markdown md dok document open read view notes plan",
    run = function(ctx)
      remote(ctx)
      local dir = cwd(ctx)
      local r = rex.exec({ bin, "list", "--json", dir }, { timeout = 10 })
      if r == nil or not r.ok then error("rdxt: couldn't list documents: " .. ((r and r.stderr) or "rdxt not found"), 0) end
      local docs = rex.json.decode(r.stdout) or {}
      local items = {}
      for _, d in ipairs(docs) do
        items[#items + 1] = { id = d.path, title = d.name, subtitle = d.dir, icon = "document",
          detail = d.recent and "recent" or nil }
      end
      if #items == 0 then error("rdxt: no .md or .dok documents here. Try rdxt: New Document.", 0) end
      local path = rex.ui.pick{ title = "Open document", placeholder = "Filter documents", items = items }
      if path == nil then return end
      show(ctx, path)
    end,
  }

  rex.action{
    name = "rdxt_new",
    title = "rdxt: New Document…",
    description = "Create a Markdown or Dok document in this folder and open it ready to edit.",
    category = "rdxt",
    keywords = "rdxt new create markdown md dok document plan notes",
    args = { name = "string?", template = "string?" },
    run = function(ctx, args)
      remote(ctx)
      local dir = cwd(ctx)
      local name = args.name or rex.ui.input{
        title = "New document in " .. basename(dir),
        placeholder = "Name, like plan or meeting-notes",
        confirm = "Next: choose a template",
      }
      if name == nil or name:match("^%s*$") then return end
      local template = args.template or rex.ui.pick{
        title = "Template for " .. name,
        items = {
          { id = "blank", title = "Blank", subtitle = "A title and nothing else", icon = "document" },
          { id = "plan", title = "Plan", subtitle = "Goal, steps as tasks, risks, open questions", icon = "branch" },
          { id = "notes", title = "Notes", subtitle = "Dated notes with a few headings", icon = "clock" },
          { id = "dok", title = "Dok case", subtitle = "SITUATION, IMPACT, GOAL, KNOWN, UNKNOWN", icon = "tag" },
        },
      }
      if template == nil then return end
      local r = rex.exec({ bin, "new", "--no-open", "--template", template, "--dir", dir, name }, { timeout = 10 })
      if r == nil or not r.ok then error("rdxt: " .. ((r and r.stderr) or "rdxt not found"), 0) end
      show(ctx, (r.stdout:gsub("%s+$", "")), { "--source" })
    end,
  }

  rex.action{
    name = "rdxt_check_config",
    title = "rdxt: Check Rex Config",
    description = "Open Rex's init.lua in rdxt, with Rex's own check of it on top. Edit it with e; saving checks it again.",
    category = "rdxt",
    keywords = "rdxt rex config init.lua lua check lint errors settings",
    run = function(ctx)
      if opts.config == nil then error("rdxt: run rdxt setup again to add this action", 0) end
      show(ctx, opts.config)
    end,
  }
end

return M
