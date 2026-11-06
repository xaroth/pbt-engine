-- Dumps Path of Building's ModCache (mod line text -> parsed modifiers) as JSON for pbt-semgen.
-- Run from a PoB checkout:
--   cd <pob>/src && LUA_PATH="../runtime/lua/?.lua;../runtime/lua/?/init.lua" \
--     luajit <this file> Data/ModCache.lua > modcache.json
local json = require("dkjson")
local cache = dofile(assert(..., "ModCache.lua path required"))

local out = {}
for text, entry in pairs(cache) do
	local mods = {}
	for _, m in ipairs(entry[1] or {}) do
		local tags = {}
		for _, tag in ipairs(m) do
			tags[#tags + 1] = tag
		end
		mods[#mods + 1] = {
			name = m.name, type = m.type, flags = m.flags, keywordFlags = m.keywordFlags,
			value = type(m.value) == "number" and m.value or nil, tags = tags,
		}
	end
	-- entry[2] is the part of the line PoB could not parse.
	out[#out + 1] = { text = text, mods = mods, unparsed = entry[2] }
end
table.sort(out, function(a, b) return a.text < b.text end)
io.write(json.encode(out))
