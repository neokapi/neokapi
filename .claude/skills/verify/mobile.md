# Verify a change to the mobile app

For a diff under `bowrain/mobile/` or a package it imports
(`packages/bowrain-client`, `kapi-format`, `contract-types`). Run the checks,
then run the development build on an iOS simulator against a local Bowrain
stack and drive the change. Read `bowrain/mobile/AGENTS.md` first. Run
everything from a checkout that has had `vp install`.

## Checks

From `bowrain/mobile`:

```bash
vpr typecheck   # 3 s
vpr test        # 10 s, jest-expo
vpr export      # 21 s, Metro resolves the workspace for iOS and Android
```

## What the machine needs

```bash
xcrun simctl list devices available | grep -E 'iPhone (16e|17e|17 Pro)'
pod --version; maestro --version
```

When one is missing, stop and ask the person to install it. Never use Expo Go:
it lacks the app's native modules. An installed development build older than
the last native change looks like a bug in yours, so compare:

```bash
git log -1 --format=%ci -- bowrain/mobile/package.json bowrain/mobile/app.config.ts
ls -ld ~/Library/Developer/CoreSimulator/Devices/*/data/Containers/Bundle/Application/*/Bowrain.app
```

## Build the development build

```bash
SIM=<udid from the list>; xcrun simctl boot "$SIM"
vpr ios --no-bundler --device "iPhone 17 Pro"   # prebuild into ios/ and pod install: 2 min
```

With Xcode 27.2 and only iOS 26 simulator runtimes, this ends with
`CommandError: No code signing certificates are available to use`: Expo takes
the simulator for a device. The prebuild and pods are done by then, so finish
with Xcode:

```bash
xcodebuild -workspace ios/Bowrain.xcworkspace -scheme Bowrain -configuration Debug \
  -destination "id=$SIM" -derivedDataPath ios/build build        # 5 min cold
xcrun simctl install "$SIM" ios/build/Build/Products/Debug-iphonesimulator/Bowrain.app
```

`ios/` is generated and gitignored. Never edit it.

## The server

Never point a build at production data. A development build signs in to
`http://localhost:8080` unless `EXPO_PUBLIC_BOWRAIN_SERVER_URL` is set when
Metro starts. The `preview` and `production` EAS profiles name the production
service: never verify with them.

```bash
make -C bowrain stack-up-e2e     # stack, password sign-in, user e2e-reviewer@example.com
```

The same compose files under another project name came up in 2.5 minutes; the
target itself was not run, because another stack held its ports. If yours
must run beside another, use `docker compose -p <name> -f compose.full.yaml -f
compose.e2e.yaml -f <override>`, where the override moves every published port,
sets `KC_HOSTNAME`, `BOWRAIN_OIDC_PUBLIC_URL` and `S3_PUBLIC_ENDPOINT` to
match, and gives the server and worker their own `image:` tags. Then run its
`keycloak-e2e` service. The realm redirects to `localhost:8080` only, so add
your port inside your Keycloak container: `kcadm.sh update clients/<id> -r
bowrain -s 'redirectUris=["http://localhost:<port>/*"]'`.

## Run and drive the app

```bash
EXPO_PUBLIC_BOWRAIN_SERVER_URL=http://localhost:8080 vpr start   # run_in_background; 6 s
```

Never set `CI=1`: Expo then stops watching files and edits never reach the
app. The background task can report failure while Metro keeps serving, so
check `lsof -nP -iTCP:8081 -sTCP:LISTEN`. A second `vpr start` takes 8082,
where the app does not look. Open the app on Metro with a scratch Maestro flow:

```yaml
appId: cloud.bowrain.app
---
- openLink: bowrain://expo-development-client/?url=http%3A%2F%2F127.0.0.1%3A8081
- runFlow: { when: { visible: "Open" }, commands: [ tapOn: "Open" ] }
- runFlow: { when: { visible: "Continue" }, commands: [ tapOn: "Continue" ] }
- runFlow:
    when: { visible: "Go home" }
    commands:
      - scrollUntilVisible: { element: "Tools button" }
      - tapOn: { point: "85%,66%" }
      - tapOn: "Close"
- extendedWaitUntil: { visible: { id: sign-in }, timeout: 90000 }
```

The first bundle takes 30 s. The flow turns off the development client's
floating Tools button, which covers the app's server cog. A source edit then
reloads in place; if a change does not show, run the flow again. Screenshot
with `xcrun simctl io "$SIM" screenshot <file>.png` and Read the file.

## The Maestro flows

`e2e/*.yaml` do not pass against a development build: all failed in 162 s on
2026-11-22. `launchApp: clearState` returns to the development client's
launcher, and on the Keycloak page Maestro sees the labels, not the
`username`, `password` and `kc-login` ids. Run patched copies, signed out and
on the sign-in screen:

```bash
T=$(mktemp -d); /bin/cp e2e/*.yaml "$T"/
sed -i '' -e '/^- launchApp:/,/clearKeychain: true/d' -e 's/id: username/text: "Email"/' \
  -e 's/id: password/text: "Password"/' -e 's/id: kc-login/text: "Sign In"/' "$T/sign-in.yaml"
maestro --device "$SIM" test -e SERVER_URL=http://localhost:8080 "$T/sign-in.yaml"   # 90 s
```

The copy passes Keycloak and stops at `workspace-list`: right after sign-in
the app requests `/api/v1/(tabs)/projects`, gets 404 and shows "Could not load
this". A relaunch shows the workspaces. Until that is fixed, no flow after
sign-in can pass. The deck flows also need a project with work waiting
(`-e WORKSPACE=… -e PROJECT=… -e LOCALE=…`), which this recipe does not seed.
`maestro test "$T"` runs the set.

## A change to a response shape

The app persists its query cache. A release build drops a cache written by
another update, except the decision log (`src/lib/queryClient.ts`, tested in
`src/__tests__/queryCacheUpdate.test.ts`). A development build stamps every
bundle `embedded`, so it always restores the old cache. Use that to test the
start-up that crashed on 2026-11-20 (#3609): run the screen on the code before
your change so the cache fills, background the app, switch to your change,
then stop and launch the app without clearing state. It must start and render.
This step was not rehearsed when the recipe was written.

## Devices, evidence, cleanup

Check a changed screen on a small phone (iPhone 16e or 17e) and a large one
(17 Pro Max), light and dark (`xcrun simctl ui "$SIM" appearance dark`). The
same `.app` installs on each; boot and install take 50 s. Record each command
and exit code, each screenshot with a line on what it shows, and each flow's
result line. Then:

```bash
kill $(lsof -tiTCP:8081 -sTCP:LISTEN); xcrun simctl shutdown "$SIM"   # yours only
make -C bowrain stack-down       # or: docker compose -p <name> ... down -v
```
