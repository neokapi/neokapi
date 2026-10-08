# Harbor Cloud overview

Harbor Cloud runs your apps on servers we manage, so you deploy with
`harbor deploy` and never patch a machine.

## Regions

Harbor Cloud runs in three regions: `eu-north`, `us-east` and `ap-south`.
Choose one with `region` in `harbor.toml`. An app runs in one region.

## Plans

| Plan | Apps | Memory per app |
| --- | --- | --- |
| Starter | 3 | 512 MB |
| Team | 20 | 4 GB |
| Scale | Unlimited | 16 GB |

## Limits

Each deploy can take up to 30 minutes to build. Harbor Cloud keeps the last 10
releases of each app, so you can return to any of them.
