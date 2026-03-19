# Heartbeat Check

On each heartbeat cycle, perform these checks:

1. Call `connector_status` for the Excalidraw project to see if there are pending changes
2. Check upstream repo with `git fetch upstream` — any new tags or commits on excalidraw/excalidraw?
3. If upstream changes found: merge and push new content to Bowrain
4. Check the Bowrain activity feed for completed translation batches
   - If QA-passed events are found: pull translations and commit to the fork
5. If any issues were encountered, file GitHub Issues and notify the team
