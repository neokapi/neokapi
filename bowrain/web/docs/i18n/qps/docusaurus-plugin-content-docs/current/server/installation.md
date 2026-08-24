---
sidebar_position: 2
title: Installation
description: Install Bowrain Server as several cooperating services — the server, worker, database, and job queue — rather than a single binary.
---

# ▒ Îñšţàļļîñĝ Ƃöŵŕàîñ Šéŕṽéŕ ▒

▒ À Ƃöŵŕàîñ đéþļöýḿéñţ îš šéṽéŕàļ çööþéŕàţîñĝ šéŕṽîçéš, ñöţ à šîñĝļé ƃîñàŕý: ▒

- ▒ **ƃöŵŕàîñ-šéŕṽéŕ** — ţĥé ŔÉŠŢ + ĝŔÞÇ ÀÞÎ (öñé þŕöçéšš; ĝŔÞÇ îš ḿüļţîþļéẋéđ öñţö ţĥé ĤŢŢÞ þöŕţ). ▒
- ▒ **ƃöŵŕàîñ-ŵöŕķéŕ** — ţĥé àšýñç ŵöŕķéŕ ţĥàţ îñĝéšţš þüšĥéš àñđ ŕüñš ţĥé àüţö-ţŕàñšļàţé-öñ-þüšĥ àüţöḿàţîöñ àĝàîñšţ àñ üþšţŕéàḿ ţŕàñšļàţîöñ þŕöṽîđéŕ. ▒
- ▒ **ÞöšţĝŕéŠǪĻ** — ţĥé àüţĥöŕîţàţîṽé šţöŕé (þŕöĵéçţš, ƃļöçķš, ŵöŕķšþàçéš, üšéŕš, ĵöƃš). Ţĥé šéŕṽéŕ ŕéǫüîŕéš ÞöšţĝŕéŠǪĻ; **ţĥéŕé îš ñö ŠǪĻîţé öŕ ƒîļé ƃàçķéñđ.** ▒
- ▒ À **ĵöƃ ǫüéüé** (Àḿàžöñ ŠǪŠ, öŕ àñ ŠǪŠ-çöḿþàţîƃļé ƃŕöķéŕ šüçĥ àš ÉļàšţîçḾǪ) àñđ **Ŕéđîš** ƒöŕ ţĥé éṽéñţ ƃüš (Ŕéđîš Šţŕéàḿš), šĥàŕéđ ƃý ţĥé šéŕṽéŕ àñđ ŵöŕķéŕ. ▒
- ▒ **ƃöŵŕàîñ-ŵéƃ** — ţĥé šţàţîç ŵéƃ ÜÎ, šéŕṽéđ àš îţš öŵñ çöñţàîñéŕ. ▒
- ▒ Àñ **ÖÎĐÇ îđéñţîţý þŕöṽîđéŕ** (é.ĝ. Ķéýçļöàķ) àñđ àñ **ŠḾŢÞ** šéñđéŕ. ▒

▒ Ţĥîš þàĝé çöṽéŕš ļöçàļ éṽàļüàţîöñ. Ƒöŕ à þŕöđüçţîöñ šţàçķ ŵîţĥ ŢĻŠ, ƃàçķüþš, àñđ à ŕéṽéŕšé þŕöẋý, šéé [Šéļƒ-Ĥöšţîñĝ](/server/self-hosting), ŵĥîçĥ îš ţĥé çàñöñîçàļ ŕéƒéŕéñçé ƒöŕ ţĥé ƒüļļ àŕçĥîţéçţüŕé àñđ ţĥé çöḿþļéţé éñṽîŕöñḿéñţ-ṽàŕîàƃļé šéţ. ▒

## ▒ Öñé-çöḿḿàñđ ļöçàļ šţàçķ ▒

▒ Ţĥé ŕéþöšîţöŕý šĥîþš à šéļƒ-çöñţàîñéđ ļöçàļ šţàçķ àţ [`ƃöŵŕàîñ/çöḿþöšé.ƒüļļ.ýàḿļ`](https://github.com/neokapi/neokapi/blob/main/bowrain/compose.full.yaml) — šéŕṽéŕ, ŵöŕķéŕ, ÞöšţĝŕéŠǪĻ, ÉļàšţîçḾǪ (ŠǪŠ-çöḿþàţîƃļé ĵöƃ ǫüéüé), Ŕéđîš, ḾîñÎÖ, Ķéýçļöàķ (ŵîţĥ à þŕé-îḿþöŕţéđ ŕéàļḿ), àñđ Ḿàîļþîţ. Îţ đéƒàüļţš ţö ţĥé öƒƒļîñé `đéḿö` ţŕàñšļàţîöñ þŕöṽîđéŕ, šö ţĥé ƒüļļ þüšĥ → ţŕàñšļàţé → þüļļ çýçļé ŵöŕķš ŵîţĥ ñö ÀÞÎ ķéýš àñđ ñö ÖÎĐÇ šéţüþ: ▒

```bash
docker compose -f bowrain/compose.full.yaml up -d --build --wait
```

▒ Éñđþöîñţš öñçé îţ îš üþ: ▒

| ▒ ÜŔĻ ▒ | ▒ Šéŕṽîçé ▒ |
| --- | --- |
| ▒ `ĥţţþ://ļöçàļĥöšţ:8080` ▒ | ▒ ƃöŵŕàîñ-šéŕṽéŕ ÀÞÎ (àñđ ţĥé ŵéƃ ÜÎ ŵîţĥ `--þŕöƒîļé ŵéƃ`) ▒ |
| ▒ `ĥţţþ://ļöçàļĥöšţ:8080/àþî/ṽ1/ĥéàļţĥ` ▒ | ▒ Ĥéàļţĥ çĥéçķ ▒ |
| ▒ `ĥţţþ://ļöçàļĥöšţ:8180` ▒ | ▒ Ķéýçļöàķ àđḿîñ çöñšöļé (àđḿîñ / àđḿîñ) ▒ |
| ▒ `ĥţţþ://ļöçàļĥöšţ:8025` ▒ | ▒ Ḿàîļþîţ (çàþţüŕéđ éḿàîļš) ▒ |

▒ Ţö šéŕṽé ţĥé ŵéƃ ÜÎ ƒŕöḿ ţĥé šéŕṽéŕ, àđđ ţĥé `ŵéƃ` þŕöƒîļé: ▒

```bash
docker compose -f bowrain/compose.full.yaml --profile web up -d --build
```

▒ Ƒöŕ ŕéàļ ţŕàñšļàţîöñš, šéţ `ƂÖŴŔÀÎÑ_ÞĻÀŢƑÖŔḾ_ÞŔÖṼÎĐÉŔ` (é.ĝ. `ĝéḿîñî`) àñđ ţĥé ḿàţçĥîñĝ ÀÞÎ ķéý îñ à `.éñṽ` ƒîļé — šéé [Šéļƒ-Ĥöšţîñĝ](/server/self-hosting#environment-variables). ▒

▒ Ţéàŕ đöŵñ ŵîţĥ `đöçķéŕ çöḿþöšé -ƒ ƃöŵŕàîñ/çöḿþöšé.ƒüļļ.ýàḿļ đöŵñ -ṽ`. ▒

## ▒ Šéļƒ-ĥöšţîñĝ ƒŕöḿ þüƃļîšĥéđ îḿàĝéš ▒

▒ Ţö ŕüñ ƒŕöḿ ţĥé þüƃļîšĥéđ `ĝĥçŕ.îö/ñéöķàþî/` îḿàĝéš àĝàîñšţ ýöüŕ öŵñ ÖÎĐÇ þŕöṽîđéŕ, üšé ţĥé ŕéƒéŕéñçé šţàçķ àţ [`ƃöŵŕàîñ/đéþļöý/đöçķéŕ/çöḿþöšé.ýàḿļ`](https://github.com/neokapi/neokapi/blob/main/bowrain/deploy/docker/compose.yaml) — Ţŕàéƒîķ, ÞöšţĝŕéŠǪĻ, ÉļàšţîçḾǪ, Ŕéđîš, ţĥé šéŕṽéŕ, ţĥé ŵöŕķéŕ, àñđ ţĥé ŵéƃ ÜÎ. ▒

```bash
docker compose -f deploy/docker/compose.yaml up -d
```

▒ Àţ ḿîñîḿüḿ, þŕöṽîđé àñ éẋţéŕñàļ ÖÎĐÇ îššüéŕ, à ĴŴŢ šéçŕéţ, àñđ (ƒöŕ àüţö-ţŕàñšļàţé) àñ üþšţŕéàḿ þŕöṽîđéŕ: ▒

```bash
POSTGRES_PASSWORD=...                          # database password
BOWRAIN_JWT_SECRET=$(openssl rand -base64 32)  # JWT signing secret
BOWRAIN_OIDC_ISSUER_URL=...                    # your realm's issuer URL
BOWRAIN_OIDC_CLIENT_SECRET=...                 # the bowrain client's secret
BOWRAIN_PLATFORM_PROVIDER=gemini               # or openai / anthropic / ollama
BOWRAIN_PLATFORM_API_KEY=...                   # provider API key
```

▒ Šéé [Šéļƒ-Ĥöšţîñĝ](/server/self-hosting) ƒöŕ ţĥé ƒüļļ þŕöđüçţîöñ ŵàļķţĥŕöüĝĥ, îñçļüđîñĝ ŢĻŠ, ƃàçķüþš, àñđ ţĥé çöḿþļéţé šéŕṽîçé ţöþöļöĝý. ▒

## ▒ Ñàţîṽé ƃîñàŕý ▒

▒ Ţĥé šéŕṽéŕ àñđ ŵöŕķéŕ àļšö šĥîþ àš ñàţîṽé ƃîñàŕîéš öñ [ĜîţĤüƃ Ŕéļéàšéš](https://github.com/neokapi/neokapi/releases). Ƃöţĥ šţîļļ ŕéǫüîŕé à ŕéàçĥàƃļé ÞöšţĝŕéŠǪĻ, þļüš à šĥàŕéđ ĵöƃ ǫüéüé (ŠǪŠ öŕ àñ ŠǪŠ-çöḿþàţîƃļé ƃŕöķéŕ) àñđ Ŕéđîš ŵĥéñ ţĥé šéŕṽéŕ àñđ ŵöŕķéŕ ŕüñ àš šéþàŕàţé þŕöçéššéš. ▒

```bash
# Linux (x86_64)
curl -LO https://github.com/neokapi/neokapi/releases/latest/download/bowrain-server-linux-amd64.tar.gz
tar xzf bowrain-server-linux-amd64.tar.gz
sudo mv bowrain-server /usr/local/bin/
```

▒ Ŕüñ ţĥé šéŕṽéŕ (ÞöšţĝŕéŠǪĻ îš ŕéǫüîŕéđ; ţĥé çöññéçţîöñ šţŕîñĝ ḿüšţ üšé ţĥé `þöšţĝŕéš://` šçĥéḿé): ▒

```bash
bowrain-server \
  --database-url postgres://bowrain:password@localhost/bowrain \
  --jwt-secret change-me-in-production \
  --oidc-issuer-url https://keycloak.example.com/realms/bowrain \
  --oidc-client-id bowrain \
  --oidc-client-secret your-client-secret \
  --port 8080
```

▒ Ţĥé šçĥéḿà îš çŕéàţéđ àüţöḿàţîçàļļý öñ ƒîŕšţ šţàŕţ; ḿîĝŕàţîöñš ŕüñ öñ šţàŕţüþ. Ţĥé àšýñç ŵöŕķéŕ îš çöñƒîĝüŕéđ éñţîŕéļý ţĥŕöüĝĥ éñṽîŕöñḿéñţ ṽàŕîàƃļéš — šéé [Çöñƒîĝüŕàţîöñ](/server/configuration). ▒

### ▒ šýšţéḿđ šéŕṽîçé ▒

▒ `/éţç/šýšţéḿđ/šýšţéḿ/ƃöŵŕàîñ-šéŕṽéŕ.šéŕṽîçé`: ▒

```ini
[Unit]
Description=Bowrain Server
After=network.target
[Service]
Type=simple
User=bowrain
Group=bowrain
Environment=BOWRAIN_DATABASE_URL=postgres://bowrain:password@localhost/bowrain
Environment=BOWRAIN_QUEUE_BACKEND=sqs
Environment=SQS_ENDPOINT=http://localhost:9324
Environment=BOWRAIN_EVENT_BACKEND=redis
Environment=BOWRAIN_REDIS_URL=redis://localhost:6379
ExecStart=/usr/local/bin/bowrain-server \
  --jwt-secret ${BOWRAIN_JWT_SECRET} \
  --oidc-issuer-url https://keycloak.example.com/realms/bowrain \
  --oidc-client-id bowrain \
  --oidc-client-secret ${BOWRAIN_OIDC_CLIENT_SECRET} \
  --port 8080
Restart=on-failure
RestartSec=5s
[Install]
WantedBy=multi-user.target
```

▒ Éñàƃļé àñđ šţàŕţ: ▒

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now bowrain-server
sudo systemctl status bowrain-server
```

## ▒ ÖÎĐÇ þŕöṽîđéŕ šéţüþ ▒

▒ Ƃöŵŕàîñ Šéŕṽéŕ ŕéǫüîŕéš àñ ÖÎĐÇ þŕöṽîđéŕ ƒöŕ àüţĥéñţîçàţîöñ. Àñý ÖÎĐÇ-çöḿþļîàñţ
þŕöṽîđéŕ ŵöŕķš (Ķéýçļöàķ, Àüţĥ0, Öķţà, Àžüŕé ÀĐ, Đéẋ). Ţĥé ļöçàļ šţàçķ àƃöṽé
îḿþöŕţš à þŕé-çöñƒîĝüŕéđ Ķéýçļöàķ ŕéàļḿ àüţöḿàţîçàļļý; ƒöŕ ýöüŕ öŵñ þŕöṽîđéŕ šéé
ţĥé [ÖÎĐÇ þŕöṽîđéŕ šéţüþ îñ Šéļƒ-Ĥöšţîñĝ](/server/self-hosting#oidc-provider-setup). ▒

## ▒ Ĥéàļţĥ çĥéçķ ▒

▒ Ṽéŕîƒý ţĥé šéŕṽéŕ îš ŕüññîñĝ: ▒

```bash
curl http://localhost:8080/api/v1/health
```

## ▒ Ñéẋţ šţéþš ▒

- ▒ [Çöñƒîĝüŕàţîöñ](/server/configuration) — ţĥé çöḿþļéţé éñṽîŕöñḿéñţ-ṽàŕîàƃļé àñđ ÇĻÎ-ƒļàĝ ŕéƒéŕéñçé. ▒
- ▒ [Ĝéţţîñĝ Šţàŕţéđ](/server/getting-started) — ƒîŕšţ ļöĝîñ, ŵöŕķšþàçéš, îñṽîţàţîöñš. ▒
- ▒ [Šéļƒ-Ĥöšţîñĝ](/server/self-hosting) — þŕöđüçţîöñ đéþļöýḿéñţ ŵîţĥ ŢĻŠ àñđ ƃàçķüþš. ▒
