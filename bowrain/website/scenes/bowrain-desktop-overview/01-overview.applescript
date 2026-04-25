-- Walkthrough: bowrain-desktop-overview (proof-of-concept)
-- Scene 1: overview — open dashboard, hold for 4 seconds.
--
-- Drives the real native Bowrain Wails app launched by
-- scripts/record-desktop-scene.sh against BOWRAIN_SERVER_URL.

-- Wait for the connect screen to disappear (auto-connect transitions
-- mode out of "connecting" once BOWRAIN_TOKEN auto-connect completes).
-- The connect screen shows "Welcome to Bowrain"; once gone, we're in
-- the dashboard regardless of whether the workspace has projects yet.
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

-- Hold the dashboard for the recording.
delay 4
