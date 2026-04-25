-- Walkthrough: bowrain-desktop-tm-explorer
-- Scene 1: browse — navigate to the Translation Memory view from the dashboard.
--
-- Drives the real native Bowrain Wails app launched by
-- scripts/record-desktop-scene.sh against BOWRAIN_SERVER_URL.

on clickButtonByName(buttonName, timeoutSeconds)
	set startTime to current date
	repeat
		if (current date) - startTime > timeoutSeconds then
			error "button '" & buttonName & "' did not appear within " & timeoutSeconds & "s"
		end if
		try
			tell application "System Events"
				tell process "Bowrain"
					set kids to entire contents of window 1
					repeat with k in kids
						try
							if (role of k is "AXButton") and (name of k is buttonName) then
								click k
								return
							end if
						end try
					end repeat
				end tell
			end tell
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

-- 2. Beat for the dashboard to render before clicking the sidebar.
delay 1

-- 3. Click the "Memory" sidebar entry to open the Translation Memory view.
clickButtonByName("Memory", 10)

-- 4. Wait for the TM landing copy.
waitForText("Select a project to explore its translation memory.", 10)

-- 5. Hold the final frame.
delay 3
