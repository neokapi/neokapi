# Bowrain mobile on TestFlight

These are the steps that take the Bowrain mobile app (`bowrain/mobile/`) from
the repository to internal testers on TestFlight. Everything that does not need
an Apple or Expo account is already in the tree. What is left needs your
accounts, so you run it yourself.

The app's design is in [bowrain-mobile-review.md](bowrain-mobile-review.md).

## What is already in place

- `bowrain/mobile/eas.json` has four build profiles. `development` builds a
  development client for the iOS Simulator; `development-device` builds it
  for a registered device; `preview` makes an internal build; `production`
  makes a store build and raises the build number on every build (the number
  lives on EAS, `appVersionSource: remote`).
- `bowrain/mobile/app.config.ts` sets the bundle identifier
  `cloud.bowrain.app`, the `bowrain` scheme, the version, the associated
  domain `app.bowrain.cloud`, and `ITSAppUsesNonExemptEncryption: false`. Its
  runtime version follows the `fingerprint` policy (see
  [Updates and new builds](#updates-and-new-builds)).
- `bowrain/apps/web/public/.well-known/apple-app-site-association` lists the
  app for `https://app.bowrain.cloud/auth/callback`. The web deploy uploads it
  as `application/json`.

## Identifiers

| Value | Where it is set |
| --- | --- |
| EAS project `@neokapi/bowrain`, ID `37119e10-8ed7-40c2-b1ea-c1fad898d0b4` | `easProjectId` and `easOwner` in `bowrain/mobile/app.config.ts` |
| App Store Connect app ID `6834545030` | `submit.production.ios.ascAppId` in `bowrain/mobile/eas.json` |
| Apple Team ID `8X6GKF24MG` | `submit.production.ios.appleTeamId` in `eas.json`, and both entries in `bowrain/apps/web/public/.well-known/apple-app-site-association` |

The association file is served at
`https://app.bowrain.cloud/.well-known/apple-app-site-association` as
`application/json`.

## Steps

1. **Enrol in the Apple Developer Program** at
   [developer.apple.com/programs](https://developer.apple.com/programs/),
   as an organisation so the store lists the company as the seller. This
   needs a D-U-N-S number and can take a few days. When it is done, copy the
   Team ID from Membership details.

2. **Create the app record** in
   [App Store Connect](https://appstoreconnect.apple.com/): Apps, then the
   plus button, then New App. Use platform iOS, name Bowrain, bundle ID
   `cloud.bowrain.app` (register it under Identifiers first if it is not in
   the list), and any SKU. Open App Information and copy the Apple ID number.

3. **Sign in to Expo.** Run the EAS CLI through `vpx`, or install it
   globally and drop the `vpx eas-cli@latest` prefix.

   ```bash
   vpx eas-cli@latest login
   vpx eas-cli@latest whoami
   ```

4. **Link the project.** From `bowrain/mobile/`:

   ```bash
   eas init
   ```

   The project is `@neokapi/bowrain`, and its ID and owner are set in
   `easProjectId` and `easOwner` in `app.config.ts`. `app.config.ts` is
   dynamic, so `eas init` cannot write to it; on a new project, copy the
   values it prints into those two constants.

5. **Build for the store.** From `bowrain/mobile/`:

   ```bash
   eas build --platform ios --profile production
   ```

   The first build asks you to sign in to your Apple account and lets EAS
   create the distribution certificate and provisioning profile. Let it. EAS
   also turns on the Associated Domains capability for the bundle ID because
   the config declares it.

6. **Submit the build.**

   ```bash
   eas submit --platform ios --profile production
   ```

   Choose the build from step 5 when asked. Apple processes it in about 10 to
   15 minutes, then it shows under TestFlight in App Store Connect. Later you
   can build and submit in one step with
   `eas build --platform ios --profile production --auto-submit`.

7. **Add internal testers.** In App Store Connect, open TestFlight, create an
   internal group, and add people. Internal testers must be users of your App
   Store Connect team, up to 100 of them, and their builds need no Beta App
   Review. Testers install the TestFlight app and accept the invitation. To
   add each new build to the group automatically, put its name in
   `submit.production.ios.groups` in `eas.json`.

8. **Answer the export compliance question.** The app sets
   `ITSAppUsesNonExemptEncryption` to `false`: it uses only the encryption
   the operating system provides for HTTPS and the keychain. App Store Connect
   then marks each build as compliant without asking. If it does ask, answer
   that the app uses no non-exempt encryption.

9. **Fill in the privacy nutrition labels** under App Privacy before external
    testing or release. A starting point, which you confirm against what the
    server stores:
    - Data used to track you: none. The app carries no analytics or
      advertising SDK.
    - Contact info, email address: collected, linked to the person, for app
      functionality (the account the person signs in with).
    - Identifiers, user ID: collected, linked, for app functionality.
    - User content, other: collected, linked, for app functionality (review
      decisions and edited text sent to the Bowrain server).
    - Privacy policy URL: required. Give the URL of the Bowrain privacy
      policy, and open it first to check that it shows the policy.

## Updates and new builds

An installed build takes an over-the-air update only when both have the same
runtime version. `app.config.ts` sets the `fingerprint` policy: the runtime
version is a hash of what shapes the native app, the native dependencies,
the config plugins and the app config. `bowrain/mobile/fingerprint.config.js`
leaves out the version numbers, `eas.json` and the config's `extra` section,
which holds JavaScript data only. EAS Build and `eas update` each compute the
hash from the checkout they run on.

To see which runtime version the checkout has, from `bowrain/mobile/`:

```bash
vp exec expo-updates runtimeversion:resolve --platform ios
```

**A JavaScript-only change** keeps the hash. Publish it to the builds that
have it:

```bash
eas update --channel production --environment production --message "<what changed>"
```

**A native change** (a native package, a config plugin, a native setting in
`app.config.ts`) gives a new hash. An update made after it reaches no build
already installed: those builds keep the last update made for their hash.
Make and submit a new build, and raise `version` in `app.config.ts` for a
store release:

```bash
eas build --platform ios --profile production --auto-submit
```

### Expo SDK updates

The packages the Expo SDK pins (`react`, `react-native`, Reanimated, the
`expo-*` packages and the rest of its `bundledNativeModules.json` set) move
together with an SDK update. Dependabot opens a pull request for `expo` alone
and never for the others (`.github/dependabot.yml`). To finish that pull
request, run, from `bowrain/mobile/` on its branch:

```bash
vp exec expo install --fix
```

It moves every pinned package to the version the new SDK expects; then run
`vp install` at the repository root and commit. The update changes the runtime
version, so it needs a new native build, and it ends over-the-air updates for
the build people have installed. Merge it when that build is ready to ship.

Updates published after the change reach people once they install the new
build from TestFlight or the App Store.

## Before external testing or release

External TestFlight groups and the App Store go through review. Two things
come up there:

- **A review account.** App Review needs a username and password for a
  workspace with work to review. Create one on the production server and give
  it in the review notes.
- **Account deletion.** Guideline 5.1.1(v) asks apps with accounts to let
  people delete them. The app signs in to accounts made on the web, so a link
  to the account page may be enough. Confirm with App Review.

The production sign-in redirect should then move from `bowrain://auth/callback`
to `https://app.bowrain.cloud/auth/callback`, once the association file is
served correctly and the server's redirect allowlist accepts that URL. See
[Sign-in and permissions](bowrain-mobile-review.md#sign-in-and-permissions).
