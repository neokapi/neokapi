-- Walkthrough: bowrain-desktop-tm-explorer
-- Scene 1: browse — navigate to the Translation Memory view from the dashboard.
--
-- Drives the real native Bowrain Wails app launched by
-- scripts/record-desktop-scene.sh against BOWRAIN_SERVER_URL. Uses
-- `cliclick` to physically move the cursor (visible in the recording);
-- AppleScript synthetic clicks send AXPress events but don't move the
-- pointer, which leaves recordings looking inert.

-- Move-then-click using cliclick. Easing factor 50 gives a smooth
-- human-paced glide; raise for slower, lower for snappier.
on moveAndClick(x, y)
	set cmd to "/opt/homebrew/bin/cliclick -e 50 m:" & x & "," & y & " w:300 c:" & x & "," & y
	do shell script cmd
end moveAndClick

-- Look up an AXButton by its `name` attribute (driven by aria-label) and
-- return its center coordinates as {centerX, centerY}.
on buttonCenter(buttonName)
	tell application "System Events"
		tell process "Bowrain"
			set kids to entire contents of window 1
			repeat with k in kids
				try
					if (role of k is "AXButton") and (name of k is buttonName) then
						set p to position of k
						set sz to size of k
						set cx to (item 1 of p) + ((item 1 of sz) div 2)
						set cy to (item 2 of p) + ((item 2 of sz) div 2)
						return {cx, cy}
					end if
				end try
			end repeat
		end tell
	end tell
	error "button '" & buttonName & "' not found"
end buttonCenter

on clickButtonByName(buttonName, timeoutSeconds)
	set startTime to current date
	repeat
		if (current date) - startTime > timeoutSeconds then
			error "button '" & buttonName & "' did not appear within " & timeoutSeconds & "s"
		end if
		try
			set coords to buttonCenter(buttonName)
			my moveAndClick(item 1 of coords, item 2 of coords)
			return
		end try
		delay 0.3
	end repeat
end clickButtonByName

on waitForText(needle, timeoutSeconds)
	set startTime to current date
	repeat
		if (current date) - startTime > timeoutSeconds then
			error "text '" & needle & "' did not appear within " & timeoutSeconds & "s"
		end if
		try
			tell application "System Events"
				tell process "Bowrain"
					set kids to entire contents of window 1
					repeat with k in kids
						try
							if role of k is "AXStaticText" then
								if value of k is needle then return
							end if
						end try
					end repeat
				end tell
			end tell
		end try
		delay 0.3
	end repeat
end waitForText

-- 1. Wait for the connect screen to disappear (auto-connect completed).
set startTime to current date
repeat
	if (current date) - startTime > 20 then error "Dashboard did not load within 20s"
	set onConnectScreen to false
	try
		tell application "System Events"
			tell process "Bowrain"
				set kids to entire contents of window 1
				repeat with k in kids
					try
						set v to value of k
						if v is "Welcome to Bowrain" then
							set onConnectScreen to true
							exit repeat
						end if
					end try
				end repeat
			end tell
		end tell
	end try
	if not onConnectScreen then exit repeat
	delay 0.5
end repeat

-- 2. Brief beat for the dashboard to settle, plus park the cursor below
--    the window so the recording starts with a "neutral" pointer.
delay 0.5
do shell script "/opt/homebrew/bin/cliclick m:600,950"
delay 0.5

-- 3. Click "Memory" in the workspace sidebar.
clickButtonByName("Memory", 10)

-- 4. Wait for the TM landing copy.
waitForText("Select a project to explore its translation memory.", 10)

-- 5. Hold the final frame.
delay 2
