# Heartbeat Check

On each heartbeat cycle, perform these checks:

1. Call `connector_status` for each project to see if there are pending changes
2. Check upstream repos with `git fetch upstream` — any new tags or commits?
3. If upstream changes found: merge and push new content to Bowrain
4. Check the Bowrain activity feed for completed translation batches
   - If QA-passed events are found: pull translations and commit to the fork
5. Check your email inbox with `email.listInbox` for messages needing a reply
6. If any issues were encountered, file GitHub Issues and notify the team via email
