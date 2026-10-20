local mp = require("mp")
local overlay = mp.create_osd_overlay("ass-events")

local function timecode(value)
    value = math.max(0, math.floor(value or 0))
    return string.format("%02d:%02d:%02d", math.floor(value / 3600), math.floor(value / 60) % 60, value % 60)
end

local function update()
    local position = mp.get_property_number("time-pos")
    local duration = mp.get_property_number("duration")
    local width, height = mp.get_osd_size()
    if not position or not duration or duration <= 0 or width <= 0 or height <= 0 then
        overlay.hidden = true
        overlay:update()
        return
    end

    overlay.hidden = false
    overlay.res_x = width
    overlay.res_y = height

    local playlist_position = mp.get_property_number("playlist-pos")
    local playlist_count = mp.get_property_number("playlist-count")
    local playlist_text = ""
    if playlist_position and playlist_count and playlist_count > 1 then
        playlist_text = string.format("  %d/%d", math.floor(playlist_position) + 1, math.floor(playlist_count))
    end
    local time_width = 24 + #playlist_text
    local bar_width = math.max(4, math.min(36, math.floor(width * 0.72) - time_width))
    local progress = math.max(0, math.min(1, position / duration))
    local filled = math.floor(progress * bar_width)
    local bar = string.rep("━", filled) .. "●" .. string.rep("─", bar_width - filled)
    local text = string.format("%s / %s%s  %s", timecode(position), timecode(duration), playlist_text, bar)
    local font_size = math.max(1, math.floor(height * 0.12))
    local y = math.max(font_size, height - font_size * 1.1)
    overlay.data = string.format(
        "{\\an8\\pos(%d,%d)\\fs%d\\bord0\\shad0\\1c&H00D7FF&}%s",
        math.floor(width / 2), math.floor(y), font_size, text
    )
    overlay:update()
end

mp.add_periodic_timer(0.5, update)
mp.register_event("file-loaded", update)
