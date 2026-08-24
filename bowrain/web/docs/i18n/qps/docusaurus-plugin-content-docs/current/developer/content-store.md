---
title: Content Store
sidebar_position: 11
---

# ▒ Çöñţéñţ Šţöŕé ▒

▒ Ţĥé Çöñţéñţ Šţöŕé þŕöṽîđéš ṽéŕšîöñéđ, çöñţéñţ-àđđŕéššàƃļé þéŕšîšţéñçé ƒöŕ çöñţéñţ. Îţ šéŕṽéš àš ţĥé çéñţŕàļ þéŕšîšţéñçé ļàýéŕ ƒöŕ ñéöķàþî þŕöĵéçţš. ▒

## ▒ Àŕçĥîţéçţüŕé ▒

▒ Ţĥé šţöŕé šîţš ƃéţŵééñ çöññéçţöŕš (ŵĥîçĥ þüļļ/þüšĥ éẋţéŕñàļ çöñţéñţ) àñđ ţĥé þŕöçéššîñĝ þîþéļîñé (ƒļöŵš, ţööļš, çöñţéñţ ḿéḿöŕý, ţéŕḿîñöļöĝý): ▒

```
Connectors → ContentStore ← → Flows/Tools
                  ↕
              Versions
```

### ▒ Ķéý Çöñçéþţš ▒

- ▒ **ƂļöçķÎđéñţîţý**: Çöñţéñţ-àđđŕéššàƃļé ĥàšĥîñĝ (ŠĤÀ-256) ƒöŕ ƃļöçķ đéđüþļîçàţîöñ àñđ çĥàñĝé đéţéçţîöñ ▒
- ▒ **ÇöñţéñţŔéƒ**: Ļîñķš ƃļöçķš ţö ţĥéîŕ éẋţéŕñàļ çöññéçţöŕ šöüŕçé ŵîţĥ šýñç ţŕàçķîñĝ ▒
- ▒ **ĐîšþļàýĤîñţ**: ÜÎ ŕéñđéŕîñĝ ĝüîđàñçé (þŕéṽîéŵ, çöñţéẋţ, ḿàẋ ļéñĝţĥ, çöñţéñţ ţýþé) ▒
- ▒ **Ṽéŕšîöñ**: Ñàḿéđ šñàþšĥöţ öƒ þŕöĵéçţ šţàţé ŵîţĥ ƃļöçķ-ļéṽéļ đîƒƒîñĝ ▒

## ▒ ÇöñţéñţŠţöŕé Îñţéŕƒàçé ▒

▒ `ÇöñţéñţŠţöŕé` (`ƃöŵŕàîñ/çöŕé/šţöŕé/šţöŕé.ĝö`) îš ţĥé üñîöñ öƒ ŕöļé
îñţéŕƒàçéš, öñé þéŕ çöñçéŕñ. Àļļ çöñţéñţ öþéŕàţîöñš àŕé **šţŕéàḿ-šçöþéđ**: àñ
éḿþţý šţŕéàḿ ñàḿé đéƒàüļţš ţö `"ḿàîñ"`, ŵĥîçĥ éṽéŕý þŕöĵéçţ îḿþļîçîţļý ĥàš. ▒

```go
// ContentStore is the primary persistence interface for content,
// the union of the role interfaces. All content operations are stream-scoped.
type ContentStore interface {
    ProjectStore    // projects: create, get, list, update, delete
    StreamStore     // streams within a project
    CollectionStore // collections (stream-scoped)
    ItemStore       // items (stream-scoped)
    BlockStore      // blocks, notes, history (stream-scoped)
    VersionStore    // named versions + diffs (stream-scoped)
    ChangeFeed      // incremental sync change log (stream-scoped)
    AssetStore      // assets and locale variants

    Close() error
}
```

▒ Ŕéþŕéšéñţàţîṽé šîĝñàţüŕéš — ñöţé ţĥé `šţŕéàḿ` þàŕàḿéţéŕ ţĥŕöüĝĥöüţ: ▒

```go
// BlockStore
StoreBlocks(ctx context.Context, projectID, stream string, blocks []*model.Block) error
GetBlocks(ctx context.Context, query BlockQuery) ([]*StoredBlock, error)

// VersionStore
CreateVersion(ctx context.Context, projectID, stream, label, description string) (*Version, error)
Diff(ctx context.Context, fromVersion, toVersion string) (*VersionDiff, error)
```

## ▒ Ƃàçķéñđš ▒

▒ Ţŵö ƃàçķéñđš îḿþļéḿéñţ `ÇöñţéñţŠţöŕé`, ŵîţĥ đîƒƒéŕéñţ ŕöļéš: ▒

- ▒ **ÞöšţĝŕéŠǪĻ** îš ţĥé šéŕṽéŕ'š öñļý ƃàçķéñđ. `ƃöŵŕàîñ-šéŕṽéŕ` ŕéƒüšéš ţö
  šţàŕţ ŵîţĥöüţ à `þöšţĝŕéš://` đàţàƃàšé ÜŔĻ àñđ ƃüîļđš àļļ öƒ îţš šţöŕéš öñ
  ţĥàţ çöññéçţîöñ. Ţĥîš îš ţĥé šöüŕçé öƒ ţŕüţĥ ƒöŕ éṽéŕý ŵöŕķšþàçé. ▒
- ▒ **ŠǪĻîţé** (`ƃöŵŕàîñ/šţöŕé/šǫļîţéšţöŕé`) ƃàçķš ţĥé đéšķţöþ àþþ'š ļöçàļ
  ŵöŕķîñĝ çöþý — à çàçĥé ƒöŕ šþééđ àñđ öƒƒļîñé éđîţš ţĥàţ ḿîŕŕöŕš ţĥé šéŕṽéŕ
  àñđ îš ñéṽéŕ à šöüŕçé öƒ ţŕüţĥ. ▒

```go
import "github.com/neokapi/neokapi/bowrain/store/sqlitestore"

store, err := sqlitestore.NewSQLiteStore("working-copy.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

▒ Ƃöţĥ ƃàçķéñđš šĥàŕé öñé ļöĝîçàļ šçĥéḿà — þŕöĵéçţš, šţŕéàḿš, çöļļéçţîöñš,
îţéḿš, ƃļöçķš, ṽéŕšîöñš, ţĥé çĥàñĝé ļöĝ, àñđ àššéţš. ▒

## ▒ Ƃļöçķ Îđéñţîţý ▒

▒ Éṽéŕý šţöŕéđ ƃļöçķ ĝéţš à çöñţéñţ-àđđŕéššàƃļé îđéñţîţý çöḿþüţéđ ƒŕöḿ îţš šöüŕçé ţéẋţ: ▒

```go
identity := model.ComputeIdentity(block)
// identity.ContentHash = SHA-256 of normalized source text
// identity.ContextHash = SHA-256 of block name, type, and properties
```

▒ Ţĥîš éñàƃļéš: ▒

- ▒ **Đéđüþļîçàţîöñ**: Îđéñţîçàļ šöüŕçé ţéẋţ šĥàŕéš ţĥé šàḿé çöñţéñţ ĥàšĥ ▒
- ▒ **Çĥàñĝé đéţéçţîöñ**: Ṽéŕšîöñ đîƒƒš çöḿþàŕé çöñţéñţ ĥàšĥéš îñšţéàđ öƒ ƒüļļ ţéẋţ ▒
- ▒ **Çàçĥé îñṽàļîđàţîöñ**: Ţŕàñšļàţîöñš çàñ ƃé çàçĥéđ ƃý çöñţéñţ ĥàšĥ ▒

## ▒ Ṽéŕšîöñ Ţŕàçķîñĝ ▒

▒ Ṽéŕšîöñš àŕé ñàḿéđ šñàþšĥöţš öƒ à þŕöĵéçţ'š ƃļöçķ šţàţé: ▒

```go
// Create a snapshot of a stream
v, err := store.CreateVersion(ctx, projectID, "main", "v1.0", "Initial release")

// List a stream's versions
versions, err := store.ListVersions(ctx, projectID, "main")

// Diff two versions
diff, err := store.Diff(ctx, v1.ID, v2.ID)
for _, change := range diff.Changes {
    fmt.Printf("%s: %s\n", change.BlockID, change.ChangeType)
}
```

## ▒ Ƒļöŵ Îñţéĝŕàţîöñ ▒

▒ Šéŕṽéŕ-šîđé ƒļöŵš ŕéàđ ƒŕöḿ àñđ ŵŕîţé ţö ţĥé çöñţéñţ šţöŕé ţĥŕöüĝĥ ţĥé ƒļöŵ
šéŕṽîçé ŕàţĥéŕ ţĥàñ à ƒļöŵ-éẋéçüţöŕ öþţîöñ — à ŕüñ ļöàđš ţĥé þŕöĵéçţ'š ƃļöçķš
ƒŕöḿ ţĥé šţöŕé, éẋéçüţéš ţĥé ƒļöŵ, àñđ šţöŕéš ţĥé þŕöđüçéđ ţàŕĝéţš ƃàçķ. Šéé
[Šéŕṽéŕ-Šîđé Ƒļöŵš](/server/flows). ▒
