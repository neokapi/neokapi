# Verify a change to a desktop app

For a diff under `apps/kapi-desktop/`, `bowrain/apps/bowrain/`, or a package a
desktop frontend imports. Run the checks, then run the app on a throwaway data
root and drive the change. Run frontends from a checkout that has had `vp
install`; `make kapi-desktop-dev` and `kapi-desktop-frontend-dev` run `vp
install` first, so never use them in a worktree that other agents share.

## Checks

```bash
make kapi-desktop-test          # 56 s warm; make bowrain-desktop-test: 19 s
export PKG_CONFIG_PATH="$(brew --prefix icu4c)/lib/pkgconfig"   # for go test by hand
(cd apps/kapi-desktop && go test -tags fts5 ./backend/ -run TestName -count=1)
```

After any change to a bound Go method, including only its doc comment,
regenerate the bindings. The comment is copied into `app.js`, so a comment
edit alone makes the committed copy stale and fails CI.

```bash
make kapi-desktop-bindings          # 4 s
make check-kapi-desktop-bindings    # "committed bindings match the Go backend"
```

The generator runs with cgo off and warns about symbols inside the Wails
module. `scripts/wails-generate.sh` counts those in one line and fails the
target on a warning about any other package, which means a bound type of
yours did not resolve. Commit what `git status` shows under
`frontend/bindings`. The Bowrain app has
`bowrain-desktop-bindings` and `check-bowrain-desktop-bindings`.

```bash
make check-wails-bindings                  # 0.2 s, both apps
cd apps/kapi-desktop/frontend && vp check && vp test     # 2 s and 39 s
vp build && (cd ../../.. && make check-kapi-desktop-bundle)   # 4 s
```

For the Bowrain app, `vp test` and `vp build` in its `frontend/`, then `make
check-bowrain-desktop-bundle` (5 s and 12 s). Its `App.test.tsx` timed out in
`beforeAll` on a loaded machine and passed idle; rerun before calling it failed.

## The sandbox

The app shares the CLI's data root, config and keychain. Never run it on
yours. From the checkout root:

```bash
D="$(cd "${TMPDIR:-/tmp}" && pwd -P)/kapi-desktop-verify"
rm -rf "$D"; mkdir -p "$D"/{home,config,desktop,data,xdg-data,cache,plugins,bin}
for s in compass northsea; do /bin/cp -R "samples/$s" "$D/home/$s"; rm -rf "$D/home/$s/.kapi/work"; done
cat > "$D/iso" <<EOF
#!/bin/bash
for v in \$(compgen -e | grep '^KAPI_'); do unset "\$v"; done
unset CLAUDECODE CLAUDE_CODE_SESSION_ID CODEX_SESSION_ID
export HOME="$D/home" KAPI_HOME_DIR="$D/home" KAPI_CONFIG_DIR="$D/config" \\
  KAPI_DESKTOP_CONFIG_DIR="$D/desktop" KAPI_DATA_DIR="$D/data" \\
  XDG_CONFIG_HOME="$D/home/.config" XDG_DATA_HOME="$D/xdg-data" XDG_CACHE_HOME="$D/cache" \\
  KAPI_PLUGINS_DIR="$D/plugins" KAPI_PLUGINS_DIR_ONLY=1 KAPI_NO_PROJECT=1 \\
  KAPI_TELEMETRY=0 DO_NOT_TRACK=1 KAPI_NO_UPDATE_CHECK=1 WBRIDGE_PORT=5175
exec "\$@"
EOF
chmod +x "$D/iso"; touch "$D/marker"
make build    # bin/kapi, for the import
for s in compass northsea; do (cd "$D/home/$s" && "$D/iso" "$OLDPWD/bin/kapi" store import ./context -p .); done
```

Start each later tool call with the `D=` line. Without the import the checks
fail with `voice profile not found`. The launcher clears the agent markers, as
`kapi-sandbox.sh` does. Never save a provider key: keys go to the real keychain.

## Run it in a browser

`cmd/wbridge` serves the real backend over HTTP and `real.html` mounts the
real frontend on it. Only the transport differs from the native app.

```bash
export PKG_CONFIG_PATH="$(brew --prefix icu4c)/lib/pkgconfig"   # also before the native build
(cd apps/kapi-desktop && go build -tags fts5 -o "$D/bin/wbridge" ./cmd/wbridge)   # 11 s
(cd "$D" && "$D/iso" "$D/bin/wbridge" > "$D/wbridge.log" 2>&1)    # run_in_background; 1 s
for s in compass northsea; do curl -s -XPOST -d "{\"method\":\"OpenProject\",\"args\":[\"$D/home/$s\"]}" http://127.0.0.1:5175/wbridge; done
(cd apps/kapi-desktop/frontend && vp dev --force --strictPort)   # run_in_background; 5 s
```

Open `http://localhost:5174/real.html` (`localhost`, the one origin the bridge
allows). Both projects open as tabs and frontend edits reload in place. After a
Go change, rebuild and restart the bridge and reload; the tabs come back from
`$D/desktop/settings.json`. Native dialogs (Open, Save As) do nothing here.

The Playwright browser is shared with other agents. Open your own tab
(`browser_tabs`, action `new`, an `isolatedContext`), select it before each
action, and check each result names your context. `browser_resize` to 1400 x
900. A relative screenshot `filename` lands in the primary checkout's
gitignored `.playwright-mcp/`. Read each screenshot before you describe it.

## The flows that have broken

- **Runs stay in their tab.** In compass, Project Settings, Flows, add Pseudo
  Translate and Run it (no model; 8 ms). The runner shows `Completed 1 files`.
  In northsea: no runner, Undo disabled, no Save bar, and `diff -rq -x .kapi
  samples/northsea "$D/home/northsea"` prints nothing.
- **Unsaved changes.** Adding the flow makes compass dirty. Closing its tab
  asks Don't Save, Cancel or Save. Save, and `diff samples/compass/kapi.yaml
  "$D/home/compass"/kapi.yaml` shows the flow.
- **Translate and `converge`** ask for an AI model although compass names the
  offline `demo` provider. Do not pick one: it bills a real model.
- **Cancel** cannot be exercised on the samples: their runs take milliseconds.
- **Save As and Open** use native dialogs; check them in the native app.
- **Closing the window with unsaved changes.** The bridge's `/close` endpoint
  asks the same guard as the native close and quit hooks. Add a flow in
  compass and in northsea, then `curl -s -XPOST
  'http://127.0.0.1:5175/close?route=window'` (or `route=quit`). It answers
  `allowed=false` and the page shows one dialog listing both. Cancel logs no
  closer. Save all writes both recipes and `$D/wbridge.log` shows `closer ran:
  window`. For a failed save, `chmod 555 "$D/home/compass"` first: the dialog
  names compass, northsea saves, and no closer runs. Discard all runs the
  closer and leaves `kapi.yaml` as it was. With nothing unsaved the endpoint
  answers `allowed=true`. A confirmed close lets every later one through, so
  restart the bridge between cases.

  The native routes need a person: in the native window, with one and then
  two unsaved tabs, click the close button, press Cmd+Q and choose Kapi, Quit.
  Each opens the same dialog; with nothing unsaved each closes at once. A quit
  with nothing unsaved can be checked without clicking: run the binary inside
  a minimal `.app` bundle with a `CFBundleIdentifier` and send `osascript -e
  'tell application id "<id>" to quit'`.

## The native window

With `frontend/dist` built:

```bash
kill $(lsof -tiTCP:5175 -sTCP:LISTEN)    # never two backends on one sandbox
(cd apps/kapi-desktop && go build -tags fts5 -o "$D/bin/KapiDesktop" .)   # 7 s
"$D/iso" "$D/bin/KapiDesktop" > "$D/native.log" 2>&1     # run_in_background; 3 s
screencapture -x "$D/native.png"; pkill -f "$D/bin/KapiDesktop"
```

It serves the production bundle, with no hot reload. `screencapture` takes the
whole screen and can raise a screen-recording prompt only the person can
answer. Clicking needs the computer-use tools; without them, ask the person to
do the native-only steps and say what they saw.

## Components, theme, and the Bowrain app

`make kapi-storybook` serves port 6007 in 6 s; open
`/iframe.html?id=<id>&viewMode=story` with an id from `/index.json`. When
styling changed, shoot light and dark (`real.html?theme=dark`) and compare with
`packages/ui/docs/page-patterns.md`.

Running Bowrain Desktop needs a Bowrain server and a session token (the
`bowrain` target in `harness/src/driver/record-desktop.ts`). This recipe does
not cover it: run its checks and say so.

## Evidence and cleanup

Record each command and exit code, and each screenshot path with a line on
what it shows. Other processes may write the real workspace, so search it for
your sandbox path rather than trust its timestamp:

```bash
find "$HOME/Library/Application Support/kapi-desktop" "$HOME/.config/kapi" -newer "$D/marker"   # nothing
grep -ac kapi-desktop-verify "$HOME/Library/Application Support/kapi/workspaces/default/workspace.db"  # 0
for p in 5174 5175 6007; do kill $(lsof -tiTCP:$p -sTCP:LISTEN) 2>/dev/null; done
pkill -f "$D/bin/KapiDesktop"; rm -rf "$D"
```
