local mp = require("mp")

local function timecode(value)
    value = math.max(0, math.floor(value or 0))
    return string.format("%02d:%02d:%02d", math.floor(value / 3600), math.floor(value / 60) % 60, value % 60)
end

local function update()
    local position = mp.get_property_number("time-pos")
    local duration = mp.get_property_number("duration")
    if not position or not duration or duration <= 0 then
        return
    end

    local progress = math.max(0, math.min(1, position / duration))
    local prefix = string.format("%s / %s  ", timecode(position), timecode(duration))
    local terminal_size = mp.get_property_native("term-size", { w = 80, h = 24 })
    local terminal_width = terminal_size.w or 80
    -- Leave the terminal's final two columns unused to avoid its pending-wrap
    -- state scrolling the status row on the next update.
    local bar_width = math.max(4, math.min(120, math.floor(terminal_width) - #prefix - 4))
    local filled = math.floor(progress * bar_width)
    local bar = string.rep("=", filled) .. ">" .. string.rep("-", bar_width - filled)
    local text = prefix .. "[" .. bar .. "]"

    -- TCT writes video frames directly to the terminal and does not synchronize
    -- them with mpv's normal status output. Update one fixed row atomically so
    -- the status never scrolls or leaves stale copies behind the video.
    io.stdout:write("\27[?2026h\27[999;1H\27[2K\27[38;5;220m" .. text .. "\27[0m\27[?2026l")
    io.stdout:flush()
end

mp.add_periodic_timer(0.5, update)
mp.register_event("file-loaded", update)
