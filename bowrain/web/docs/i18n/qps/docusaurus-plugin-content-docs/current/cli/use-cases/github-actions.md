---
title: GitHub Actions
sidebar_label: GitHub Actions
---

# ▒ Üšé Çàšé: ķàþî îñ ĜîţĤüƃ Àçţîöñš ▒

▒ Ţĥîš ĝüîđé šĥöŵš ĥöŵ ţö üšé ķàþî (ŵîţĥ ţĥé ƃöŵŕàîñ þļüĝîñ) îñ ĜîţĤüƃ Àçţîöñš ŵöŕķƒļöŵš ƒöŕ àüţöḿàţéđ ţŕàñšļàţîöñ, ǫüàļîţý çĥéçķš, àñđ šéŕṽéŕ šýñç. ▒

## ▒ Öṽéŕṽîéŵ ▒

▒ Ţĥé [`šéţüþ-ķàþî`](https://github.com/neokapi/setup-kapi) ĜîţĤüƃ Àçţîöñ îñšţàļļš ķàþî öñ àñý ŕüññéŕ, ŵîţĥ ţĥé ƃöŵŕàîñ þļüĝîñ îñçļüđéđ ƃý đéƒàüļţ. Îţ ĥàñđļéš þļàţƒöŕḿ đéţéçţîöñ, çĥéçķšüḿ ṽéŕîƒîçàţîöñ, ƃîñàŕý çàçĥîñĝ, àñđ öþţîöñàļ šéŕṽéŕ àüţĥéñţîçàţîöñ — šö ýöüŕ ŵöŕķƒļöŵ šţéþš çàñ ƒöçüš öñ ţĥé çöñţéñţ ŵöŕķ. ▒

▒ Ţĥîš þàĝé îš ţĥé đééþ ĜîţĤüƃ Àçţîöñš ĝüîđé. Ƒöŕ ţĥé ḿàþ öƒ éṽéŕý ÇÎ àñđ
đéļîṽéŕý šüŕƒàçé — ĜîţĻàƃ ÇÎ, çöñţàîñéŕ ŕüññéŕš, àñđ ţĥé ñö-þîþéļîñé Ƃöŵŕàîñ
ĜîţĤüƃ Àþþ — šţàŕţ àţ [Ţĥé ļööþ îñ ÇÎ](/cli/ci/overview). ▒

## ▒ Šéţüþ ▒

▒ Àđđ `ñéöķàþî/šéţüþ-ķàþî@ṽ1` ţö ýöüŕ ŵöŕķƒļöŵ: ▒

```yaml
steps:
  - uses: actions/checkout@v4

  - uses: neokapi/setup-kapi@v1
```

▒ Ţĥé àçţîöñ đöŵñļöàđš ţĥé çöŕŕéçţ ƃîñàŕý ƒöŕ ţĥé ŕüññéŕ þļàţƒöŕḿ (Ļîñüẋ, ḿàçÖŠ, öŕ Ŵîñđöŵš), ṽéŕîƒîéš îţš ŠĤÀ-256 çĥéçķšüḿ, àñđ àđđš îţ ţö `ÞÀŢĤ`. Ţĥé ƃüîļţ-îñ ŵöŕķƒļöŵ ţöķéñ çöṽéŕš þüƃļîç ŕéļéàšé đöŵñļöàđš, šö ñö `ţöķéñ` îñþüţ îš ŕéǫüîŕéđ. Öñ šüƃšéǫüéñţ ŕüñš, ţĥé ƃîñàŕý îš ŕéšţöŕéđ ƒŕöḿ çàçĥé. ▒

### ▒ Àçţîöñ Îñþüţš ▒

| Input        | Description                                                | Default  |
| ------------ | ---------------------------------------------------------- | -------- |
| `version`    | CLI version (e.g. `1.1.0` or `latest`)                     | `latest` |
| `plugins`    | Comma or newline-separated plugin refs to install, as the registry names them (`''` to install nothing) | `bowrain` |
| `auth-token` | Bowrain server JWT (exported as `BOWRAIN_AUTH_TOKEN`)      | `""`     |
| `server`     | Bowrain server URL (exported as `BOWRAIN_SERVER_URL`) — self-hosted only; the hosted service is the default | `""`     |

### ▒ Àçţîöñ Öüţþüţš ▒

| Output      | Description                      |
| ----------- | -------------------------------- |
| `version`   | Installed version (e.g. `1.1.0`) |
| `cache-hit` | Whether the plugin cache was hit |

## ▒ Ŕéçöḿḿéñđéđ: Çàţçĥ üþ ŵîţĥ `ķàþî-àçţîöñ` ▒

▒ Ţĥé šîḿþļéšţ ÇÎ þàţţéŕñ üšéš ţŵö àçţîöñš ţöĝéţĥéŕ: ▒

- ▒ [`ñéöķàþî/šéţüþ-ķàþî`](https://github.com/neokapi/setup-kapi) — îñšţàļļš ķàþî (ţĥé ƃöŵŕàîñ þļüĝîñ îš îñçļüđéđ ƃý đéƒàüļţ) ▒
- ▒ [`ñéöķàþî/ķàþî-àçţîöñ`](https://github.com/neokapi/kapi-action) — ŕüñš à `ķàþî` çöḿḿàñđ (ĥéŕé, `ķàþî üþ`) àñđ çöḿḿîţš ţŕàñšļàţîöñš ▒

```yaml
name: Catch up translations

on:
  workflow_dispatch:
  push:
    branches: [main]
    paths:
      - "src/locales/en/**"

permissions:
  contents: write

jobs:
  up:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://dev.bowrain.cloud

      - uses: neokapi/kapi-action@v1
        id: up
        with:
          command: up

      - name: Summary
        if: steps.up.outputs.committed == 'true'
        run: echo "Translations committed at ${{ steps.up.outputs.commit-sha }}"
```

▒ Ŵîţĥ `çöḿḿàñđ: üþ` (ţĥé đéƒàüļţ), ţĥé àçţîöñ ŕüñš `ķàþî üþ` — ţĥé ķàþî ļööþ öñ ţĥé šéŕṽéŕ (þüšĥ → çàţçĥ üþ → þüļļ) — ţĥéñ çĥéçķš ƒöŕ çĥàñĝéš, çöḿḿîţš, àñđ þüšĥéš. À ŕüñ ţĥàţ **çàüĝĥţ üþ** (`çöñṽéŕĝéđ` — éṽéŕý ĝàţéđ šçöþé çļéàŕéđ îţš šĥîþ ĝàţé) çöḿḿîţš ţĥé þŕöđüçéđ ţŕàñšļàţîöñš; à ŕüñ ţĥàţ **þàŕķéđ** (ŵöŕķ ŕéḿàîñš ţĥàţ ñééđš à þéŕšöñ) çöḿḿîţš ŵĥàţ đîđ çàţçĥ üþ àñđ àññöţàţéš ţĥé þàŕķéđ ļöçàļéš; à **ƒàîļéđ** ŕüñ éẋîţš ñöñ-žéŕö àñđ çöḿḿîţš ñöţĥîñĝ. Îţ šéţš öüţþüţš ýöü çàñ üšé îñ šüƃšéǫüéñţ šţéþš: ▒

| Output           | Description                                                        |
| ---------------- | ------------------------------------------------------------------ |
| `status`         | `success`, `no-changes`, or `failed`                               |
| `outcome`        | With `command: up`: `converged` or `parked`                        |
| `passes`         | With `command: up`: how many reconciliation passes the run took    |
| `parked-locales` | With `command: up`: comma-separated locales still short of their gate |
| `committed`      | `true` if a commit was created                                     |
| `commit-sha`     | SHA of the created commit                                          |

### ▒ ķàþî-àçţîöñ Îñþüţš ▒

| Input            | Default                                 | Description                              |
| ---------------- | --------------------------------------- | ---------------------------------------- |
| `command`        | `up`                                    | The `kapi` command to run                |
| `args`           | `""`                                    | Additional arguments                     |
| `project`        | `""`                                    | Path to the `kapi.yaml` recipe (`-p` flag)   |
| `plan`           | `false`                                 | With `command: up`, dry-run instead (`kapi up --plan`): report pending work, memory reuse, and a token estimate — no writes, no provider calls. Pairs with `pr-comment` to post the cost of a change on its PR |
| `fail-on-parked` | `false`                                 | With `command: up`, fail the workflow when the run parks instead of committing partial progress |
| `commit`         | `true`                                  | Whether to commit changes                |
| `commit-message` | `chore: update translations via kapi`   | Commit message                           |
| `create-pull-request` | `false`                            | Deliver the changes as a branch and pull request instead of committing to the current branch |
| `pr-comment`     | `false`                                 | On pull-request events, post one sticky comment with the report — plan, `kapi up` outcome, or gate result — that re-runs update in place |
| `git-user-name`  | `Kapi Bot`                              | Git committer name                       |
| `git-user-email` | `bot@kapi.dev`                          | Git committer email                      |
| `paths`          | `""` (all changes)                      | Space-separated paths to stage for commit |

▒ :::ñöţé
Ţĥé ŵöŕķƒļöŵ ñééđš `þéŕḿîššîöñš: çöñţéñţš: ŵŕîţé` ƒöŕ ţĥé àçţîöñ ţö þüšĥ
çöḿḿîţš — þļüš `þüļļ-ŕéǫüéšţš: ŵŕîţé` ŵĥéñ `çŕéàţé-þüļļ-ŕéǫüéšţ` öŕ
`þŕ-çöḿḿéñţ` îš üšéđ.
::: ▒

## ▒ Ŕéüšàƃļé ŵöŕķƒļöŵš ▒

▒ Ƒöŕ öñé-ļîñé àđöþţîöñ, [`ñéöķàþî/ķàþî-ŵöŕķƒļöŵš`](https://github.com/neokapi/kapi-workflows)
þàçķàĝéš ţĥé çĥéçķöüţ → `šéţüþ-ķàþî` → `ķàþî-àçţîöñ` šéǫüéñçé àš ŕéüšàƃļé
ŵöŕķƒļöŵš. `üþ.ýḿļ@ṽ1` çàţçĥéš ţĥé þŕöĵéçţ üþ àñđ đéļîṽéŕš à þüļļ ŕéǫüéšţ: ▒

```yaml
name: Translations

on:
  schedule:
    - cron: "0 6 * * 1-5"
  workflow_dispatch:

jobs:
  up:
    uses: neokapi/kapi-workflows/.github/workflows/up.yml@v1
    permissions:
      contents: write
      pull-requests: write
    with:
      server: https://dev.bowrain.cloud
    secrets:
      bowrain-auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
```

▒ Öñ à šéŕṽéŕ-çöññéçţéđ þŕöĵéçţ ţĥé `ƃöŵŕàîñ-àüţĥ-ţöķéñ` šéçŕéţ àñđ `šéŕṽéŕ`
îñþüţ àŕé àļļ ţĥé ĵöƃ ñééđš — ţĥé ļööþ ŕüñš öñ ţĥé Ƃöŵŕàîñ šéŕṽéŕ. À
ļöçàļ-ṽéñüé ŕüñ þàššéš ţĥé `àñţĥŕöþîç-àþî-ķéý` šéçŕéţ îñšţéàđ. Ţĥé ŵöŕķƒļöŵ
éẋþöšéš `öüţçöḿé`, `þàššéš`, `þàŕķéđ-ļöçàļéš`, àñđ `þüļļ-ŕéǫüéšţ-üŕļ` àš ĵöƃ
öüţþüţš. ▒

▒ `ĝàţé.ýḿļ@ṽ1` îš ţĥé ḿéŕĝé ĝàţé — îţ ŕüñš `ķàþî çĥéçķ --šĥîþ`, ƒàîļš ţĥé ĵöƃ
öñ éẋîţ `3`, àñđ þöšţš öñé šţîçķý ŕéþöŕţ çöḿḿéñţ öñ ţĥé þüļļ ŕéǫüéšţ: ▒

```yaml
name: Ship gate

on:
  pull_request:
    paths:
      - "src/locales/**"
      - "kapi.yaml"
      - ".kapi/**"

jobs:
  ship-gate:
    uses: neokapi/kapi-workflows/.github/workflows/gate.yml@v1
    permissions:
      contents: read
      pull-requests: write
```

▒ Üšé ţĥé àçţîöñš đîŕéçţļý (àš îñ ţĥé éẋàḿþļéš ƃéļöŵ) ŵĥéñ ţĥé ĵöƃ ñééđš à
çüšţöḿ šĥàþé. ▒

## ▒ Éẋàḿþļé: Šĥîþ Ĝàţé öñ Þüļļ Ŕéǫüéšţ ▒

▒ Ĝàţé þüļļ ŕéǫüéšţš öñ ţĥé þŕöĵéçţ'š ŕéļéàšé ƃàŕ ŵĥéñéṽéŕ çöñţéñţ ƒîļéš
çĥàñĝé. `ķàþî çĥéçķ --šĥîþ` ŕüñš ţĥé þŕöĵéçţ'š ƃöüñđ ǫüàļîţý ĝàţéš (ƃŕàñđ,
ţéŕḿîñöļöĝý, ǪÀ) þļüš îţš šĥîþ/šöüŕçé çöṽéŕàĝé ĝàţéš, àñđ éẋîţš `3` — ƒàîļîñĝ
ţĥé ĵöƃ — ŵĥéñ àñý ĝàţé îš üñḿéţ: ▒

```yaml
name: Ship gate

on:
  pull_request:
    paths:
      - "src/locales/**"
      - "kapi.yaml"
      - ".kapi/**"

jobs:
  ship-gate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1

      - name: Enforce the ship gates
        run: kapi check --ship
```

▒ Öŕđîñàŕý ƃüîļđš ñéṽéŕ ƒàîļ öñ ţàŕĝéţ-ļàñĝüàĝé đŕîƒţ — à ļöçàļé ţĥàţ îš ƃéĥîñđ
îš þéñđîñĝ ŵöŕķ, ñöţ àñ éŕŕöŕ. `çĥéçķ --šĥîþ` îš ţĥé éẋþļîçîţ, öþţ-îñ
éñƒöŕçéḿéñţ þöîñţ. ▒

## ▒ Éẋàḿþļé: Þüšĥ šöüŕçé öñ Þüšĥ ţö Ḿàîñ ▒

▒ Šéñđ šöüŕçé çĥàñĝéš ţö Ƃöŵŕàîñ Çļöüđ ŵĥéñ ţĥéý ļàñđ öñ `ḿàîñ`. Þüšĥ îš þüŕé
ţŕàñšþöŕţ; ŵîţĥ `ƃöŵŕàîñ.çöñṽéŕĝé: öñ-þüšĥ` ţĥé šéŕṽéŕ çàţçĥéš ţĥé þŕöĵéçţ üþ öñ îţš öŵñ çļöçķ.
Üšé `ķàþî üþ` îñšţéàđ öƒ `ķàþî þüšĥ` îƒ ýöü ŵàñţ ÇÎ ţö ŵàţçĥ ţĥé ŕüñ àñđ çöḿḿîţ
ţĥé ŕéšüļţš ƃàçķ: ▒

```yaml
name: Sync Translations

on:
  push:
    branches: [main]
    paths:
      - "src/locales/**"

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://dev.bowrain.cloud

      - name: Push to Bowrain Cloud
        run: kapi push
```

▒ Ţĥé `àüţĥ-ţöķéñ` àñđ `šéŕṽéŕ` îñþüţš éẋþöŕţ `ƂÖŴŔÀÎÑ_ÀÜŢĤ_ŢÖĶÉÑ` àñđ `ƂÖŴŔÀÎÑ_ŠÉŔṼÉŔ_ÜŔĻ` àš éñṽîŕöñḿéñţ ṽàŕîàƃļéš, ŵĥîçĥ ţĥé ÇĻÎ þîçķš üþ àüţöḿàţîçàļļý. ▒

## ▒ Éẋàḿþļé: Šçĥéđüļéđ çàţçĥ-üþ ▒

▒ Çàţçĥ üþ öñ à šçĥéđüļé (é.ĝ. ñîĝĥţļý) ţö ķééþ ţàŕĝéţ ļöçàļéš üþ ţö đàţé.
`ķàþî-àçţîöñ` ŕüñš `ķàþî üþ` àñđ ĥàñđļéš ţĥé çöḿḿîţ, šö ñö ḿàñüàļ ĝîţ þļüḿƃîñĝ
îš ñééđéđ: ▒

```yaml
name: Nightly catch-up

on:
  schedule:
    - cron: "0 2 * * *" # 2 AM UTC

permissions:
  contents: write

jobs:
  up:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://dev.bowrain.cloud

      - uses: neokapi/kapi-action@v1
```

▒ Ţö ţŕàñšļàţé šþéçîƒîç ƒîļéš àđ ĥöç îñšţéàđ öƒ çàţçĥîñĝ à þŕöĵéçţ üþ, `ķàþî
ţŕàñšļàţé` ţàķéš éẋþļîçîţ îñþüţš: `ķàþî ţŕàñšļàţé šŕç/ļöçàļéš/éñ/àþþ.ĵšöñ
--ţàŕĝéţ-ļàñĝ ƒŕ` (àñ ÀÎ þŕöṽîđéŕ ķéý šüçĥ àš `ÀÑŢĤŔÖÞÎÇ_ÀÞÎ_ĶÉÝ` ḿüšţ ƃé šéţ
ƒöŕ à ÇÎ ŕüñ ţĥàţ þŕöđüçéš ţŕàñšļàţîöñš). ▒

## ▒ Éẋàḿþļé: Þüļļ àñđ Ḿéŕĝé Šéŕṽéŕ Çĥàñĝéš ▒

▒ Þüļļ ţŕàñšļàţîöñš ƒŕöḿ Ƃöŵŕàîñ Çļöüđ àñđ öþéñ à ÞŔ: ▒

```yaml
name: Pull Translations

on:
  workflow_dispatch:
  schedule:
    - cron: "0 8 * * 1" # Monday 8 AM UTC

jobs:
  pull:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://dev.bowrain.cloud

      - name: Pull from Bowrain Cloud
        run: kapi pull

      - name: Create PR if changed
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          git diff --quiet && exit 0
          BRANCH="bowrain/pull-translations-$(date +%Y%m%d)"
          git checkout -b "${BRANCH}"
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add -A
          git commit -m "chore: pull translations from Bowrain Cloud"
          git push -u origin "${BRANCH}"
          gh pr create \
            --title "Pull translations from Bowrain Cloud" \
            --body "Automated pull of latest translations from Bowrain Cloud."
```

## ▒ Àüţĥéñţîçàţîöñ ▒

▒ ķàþî šüþþöŕţš ţŵö àüţĥéñţîçàţîöñ ḿéţĥöđš îñ ÇÎ: ▒

| Method                   | How                                    | Best For                                |
| ------------------------ | -------------------------------------- | --------------------------------------- |
| **Environment variable** | Set `BOWRAIN_AUTH_TOKEN`               | GitHub Actions (via `auth-token` input) |
| **Device flow**          | Run `kapi auth login` interactively | Local development                       |

▒ Ţĥé `àüţĥ-ţöķéñ` îñþüţ öñ ţĥé šéţüþ àçţîöñ îš ţĥé šîḿþļéšţ àþþŕöàçĥ — îţ éẋþöŕţš ţĥé ţöķéñ àš `ƂÖŴŔÀÎÑ_ÀÜŢĤ_ŢÖĶÉÑ`, ŵĥîçĥ ţĥé ÇĻÎ çĥéçķš ƃéƒöŕé ļööķîñĝ ƒöŕ šţöŕéđ çŕéđéñţîàļš. ▒

### ▒ Ĝéñéŕàţîñĝ à ÇÎ Ţöķéñ ▒

▒ Çŕéàţé àñ ÀÞÎ ţöķéñ üšîñĝ ķàþî: ▒

```bash
kapi auth login                               # authenticate with Bowrain Cloud
kapi auth token create --name "CI" --expire-days 90
```

▒ Ţĥé ţöķéñ (`ƃŵţ_...`) îš šĥöŵñ öñçé — šţöŕé îţ îḿḿéđîàţéļý àš à ĜîţĤüƃ Àçţîöñš šéçŕéţ: ▒

```bash
gh secret set BOWRAIN_AUTH_TOKEN --repo your-org/your-repo
```

▒ Ýöü çàñ ļîšţ àñđ ŕéṽöķé ţöķéñš ŵîţĥ `ķàþî àüţĥ ţöķéñ ļîšţ` àñđ `ķàþî àüţĥ ţöķéñ đéļéţé`. ▒

## ▒ Þļüĝîñš ▒

▒ Ţĥé `þļüĝîñš` îñþüţ đéƒàüļţš ţö `ƃöŵŕàîñ` — ţĥé þļüĝîñ ţĥàţ þŕöṽîđéš šýñç, þüšĥ, àñđ þüļļ. Ļîšţ ŕéƒš (àš ţĥé ŕéĝîšţŕý ñàḿéš ţĥéḿ) ţö àđđ öţĥéŕš àļöñĝšîđé îţ, öŕ þàšš `''` ţö îñšţàļļ ñöţĥîñĝ: ▒

```yaml
- uses: neokapi/setup-kapi@v1
  with:
    plugins: |
      bowrain
      okapi-bridge
```

▒ Þļüĝîñš àŕé çàçĥéđ ƃéţŵééñ ŕüñš. Ţĥé çàçĥé ķéý îñçļüđéš à ĥàšĥ öƒ ţĥé þļüĝîñ ļîšţ, šö çĥàñĝéš ţö ţĥé ļîšţ ţŕîĝĝéŕ à ƒŕéšĥ îñšţàļļ. ▒

## ▒ Þîññîñĝ Ṽéŕšîöñš ▒

▒ Þîñ ţĥé ÇĻÎ ṽéŕšîöñ ţö àṽöîđ šüŕþŕîšéš ƒŕöḿ ñéŵ ŕéļéàšéš: ▒

```yaml
- uses: neokapi/setup-kapi@v1
  with:
    version: "1.1.0"
```

▒ Üšé `ļàţéšţ` (ţĥé đéƒàüļţ) ƒöŕ ŵöŕķƒļöŵš ŵĥéŕé ýöü àļŵàýš ŵàñţ ţĥé ñéŵéšţ ŕéļéàšé. ▒

## ▒ Ŕéļàţéđ ▒

- ▒ [Ţĥé ļööþ îñ ÇÎ](/cli/ci/overview) — éṽéŕý ÇÎ àñđ đéļîṽéŕý šüŕƒàçé, ţĥé éẋîţ-çöđé çöñţŕàçţ, ÇÎ àüţĥéñţîçàţîöñ ▒
- ▒ [ÇĻÎ Öṽéŕṽîéŵ](/cli/overview) ▒
- ▒ [Ƒļöŵ Ĥööķš](/cli/flows/hooks) ▒
- ▒ [ķàþî üþ](/cli/commands/up) — ŕüñ ţĥé ķàþî ļööþ öñ ţĥé šéŕṽéŕ (þüšĥ → çàţçĥ üþ → þüļļ) ▒
- ▒ [ķàþî þüšĥ](/cli/commands/push) àñđ [ķàþî þüļļ](/cli/commands/pull) ▒
- ▒ [ķàþî àüţĥ](/cli/commands/auth) ▒
- ▒ [Šöüŕçé Ļàñĝüàĝé Þŕéþàŕàţîöñ](/cli/use-cases/source-prep) — ǪÀ öñ šöüŕçé çöñţéñţ îñ ÇÎ ▒
