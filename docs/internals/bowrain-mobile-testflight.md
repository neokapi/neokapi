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
  `cloud.bowrain.app`, the `bowrain` scheme, version `0.1.0`, the associated
  domain `app.bowrain.cloud`, and `ITSAppUsesNonExemptEncryption: false`.
- `bowrain/apps/web/public/.well-known/apple-app-site-association` lists the
  app for `https://app.bowrain.cloud/auth/callback`. The web deploy uploads it
  as `application/json`.

## Placeholders you fill

Search for `FOUNDER` to find them all.

| Where | Value | Comes from |
| --- | --- | --- |
| `bowrain/mobile/app.config.ts`, `easProjectId` | EAS project ID | `eas init` |
| `bowrain/mobile/app.config.ts`, `easOwner` | Expo account or organisation | `eas init` |
| `bowrain/mobile/eas.json`, `submit.production.ios.ascAppId` | The app's Apple ID, a number | App Store Connect, App Information |
| `bowrain/mobile/eas.json`, `submit.production.ios.appleTeamId` | Team ID, 10 characters | developer.apple.com, Membership details |
| `bowrain/apps/web/public/.well-known/apple-app-site-association`, both `appIDs` entries | Team ID, as `<TEAM_ID>.cloud.bowrain.app` | Same as above |

## Before the first build

1. **Replace the icon and splash artwork.** `assets/images/icon.png` and
   `splash-icon.png` are still the Expo template's images. The Bowrain mark
   is at `bowrain/assets/brand/mark-1024.png`, but that directory is under the
   AGPL licence and `bowrain/mobile/` is Apache-2.0, so copying it is your
   call. The icon must be a 1024 by 1024 PNG. Transparency is flattened onto
   white at build time.
2. **Fix the web host for the association file.** CloudFront rewrites every
   path without a file extension to `index.html`, so today
   `https://app.bowrain.cloud/.well-known/apple-app-site-association` returns
   the web app as `text/html`. The `spa_router` function in
   `bowrain-infra/modules/spa-site/main.tf` has to pass `/.well-known/` paths
   through to S3. Until that ships, universal links do not work. Sign-in
   still works through the `bowrain://` redirect.

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

3. **Install the EAS CLI and sign in to Expo.**

   ```bash
   npm install --global eas-cli
   eas login
   eas whoami
   ```

4. **Link the project.** From `bowrain/mobile/`:

   ```bash
   eas init
   ```

   `app.config.ts` is dynamic, so `eas init` cannot write to it. It prints
   the values to add instead. Put the project ID in `easProjectId` and the
   owner in `easOwner`.

5. **Fill the remaining placeholders** from the table above: `ascAppId` and
   `appleTeamId` in `eas.json`, and the Team ID in the association file.
   Commit the change and deploy the web app so the file is live.

6. **Build for the store.** From `bowrain/mobile/`:

   ```bash
   eas build --platform ios --profile production
   ```

   The first build asks you to sign in to your Apple account and lets EAS
   create the distribution certificate and provisioning profile. Let it. EAS
   also turns on the Associated Domains capability for the bundle ID because
   the config declares it.

7. **Submit the build.**

   ```bash
   eas submit --platform ios --profile production
   ```

   Choose the build from step 6 when asked. Apple processes it in about 10 to
   15 minutes, then it shows under TestFlight in App Store Connect. Later you
   can build and submit in one step with
   `eas build --platform ios --profile production --auto-submit`.

8. **Add internal testers.** In App Store Connect, open TestFlight, create an
   internal group, and add people. Internal testers must be users of your App
   Store Connect team, up to 100 of them, and their builds need no Beta App
   Review. Testers install the TestFlight app and accept the invitation. To
   add each new build to the group automatically, put its name in
   `submit.production.ios.groups` in `eas.json`.

9. **Answer the export compliance question.** The app sets
   `ITSAppUsesNonExemptEncryption` to `false`: it uses only the encryption
   the operating system provides for HTTPS and the keychain. App Store Connect
   then marks each build as compliant without asking. If it does ask, answer
   that the app uses no non-exempt encryption.

10. **Fill in the privacy nutrition labels** under App Privacy before external
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
