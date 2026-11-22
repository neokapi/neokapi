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

With Xcode 27.2 this ends with `CommandError: No code signing certificates are
available to use`, with `--device` and without it. Xcode 27's `xcrun devicectl
list devices` lists the simulators too, and Expo CLI 58 reads every entry
there as a physical device, so the simulator is built for a phone. The cause
is in Expo CLI, not in this repository. The prebuild and pods are done by
then, so finish with Xcode:

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
where the app does not look. Open the app on Metro from a clean install:

```bash
maestro --device "$SIM" test e2e/subflows/launch.yaml   # 30 s for the first bundle
```

The flow clears the app's state, opens the development client's link to
Metro, and turns off the development menu's onboarding sheet and floating
Tools button, which covers the app's server cog. A source edit then reloads
in place; if a change does not show, run the flow again. Screenshot with
`xcrun simctl io "$SIM" screenshot <file>.png` and Read the file.

## The Maestro flows

Run the committed flows for every change under `bowrain/mobile/`. They run
here and nowhere else: no CI job runs them. With the stack and Metro up, from
`bowrain/mobile`:

```bash
maestro --device "$SIM" test $(node e2e/seed.mjs) e2e/   # 6 flows, 7 min
```

`e2e/seed.mjs` makes a project in the test user's workspace whose strings a
second account has translated into `nb`, and prints the `-e WORKSPACE=… -e
PROJECT=… -e LOCALE=…` the deck flows take. A reviewer may not approve their
own translation, which is why a second account writes them. It signs both in
through the local stack's device flow and refuses any server but a local one.
Each flow starts from a clean install (`e2e/subflows/launch.yaml`) and signs
in through the real Keycloak. Record each flow's result line.

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
