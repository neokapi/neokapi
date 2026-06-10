---
sidebar_position: 4
title: Installation
description: Install the kapi CLI on macOS, Linux, or Windows via Homebrew, WinGet, or a direct binary download. Offline by default; no configuration needed to start.
keywords: [kapi, install, homebrew, winget, binary download, macos, linux, windows]
---

# Installering

neokapi leveres som to artefakter du kan installere uavhengig av hverandre:

- **`kapi`-CLI-en** — én selvstendig binærfil som kjører frakoblet som standard
  og arbeider direkte på filene dine;
- **Kapi Desktop** — den visuelle følgeappen, som inkluderer CLI-en.

De to seksjonene under dekker hver av dem. Vil du bare ha kommandolinjen,
trenger du kun den første seksjonen.

## Installer Kapi CLI

Når du har [prøvd kapi i nettleseren](/kapi/get-started/quickstart), installer
binærfilen for å kjøre den lokalt mot dine egne filer.

### Homebrew (macOS/Linux)

```bash
brew install neokapi/tap/kapi
```

### WinGet (Windows)

```powershell
winget install Neokapi.Kapi
```

### Binærnedlastinger

Forhåndsbygde binærfiler for alle plattformer er tilgjengelige på
[GitHub Releases](https://github.com/neokapi/neokapi/releases)-siden:

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

### Fra kildekode (Go-utviklere)

Installer siste versjon med Go:

```bash
go install github.com/neokapi/neokapi/kapi/cmd/kapi@latest
```

Eller bygg kodelageret:

```bash
git clone https://github.com/neokapi/neokapi.git
cd neokapi
make build       # Build kapi CLI → bin/kapi
```

### Verifiser installasjonen

```bash
kapi version
```

### Legg til leverandørlegitimasjon (valgfritt)

De regelbaserte kommandoene — pseudooversettelse, ordtelling, merkevaresjekker
mot en profilfil — trenger ingen legitimasjon. For LLM-basert oversettelse,
kvalitetskontroll og gjennomgang lagrer du en leverandørnøkkel én gang under et
navn du refererer til i flyter:

```bash
kapi credentials add my-openai --provider openai --api-key sk-…
kapi credentials list       # see what's saved
```

Legitimasjon ligger i operativsystemets nøkkelring. Se
[hurtigstarten](/kapi/get-started/quickstart) for hva du kan kjøre videre.

## Installer Kapi Desktop

Kapi Desktop er den visuelle følgesvennen til CLI-en. Hver pakke under
installerer `kapi`-CLI-en som en avhengighet, så én enkelt installasjon dekker
begge. Se [Kapi Desktop-oversikten](/kapi/desktop/overview) for hva den gjør.

### macOS (Homebrew)

```bash
brew install --cask neokapi/tap/kapi
```

### Windows (installasjonsprogram)

Last ned og kjør det signerte installasjonsprogrammet fra
[GitHub Releases](https://github.com/neokapi/neokapi/releases):

- **amd64**: `kapi-desktop-X.Y.Z-windows-amd64-setup.exe`
- **arm64**: `kapi-desktop-X.Y.Z-windows-arm64-setup.exe`

Installasjonsprogrammet er Authenticode-signert og registrerer en
startmenyoppføring og et avinstalleringsprogram.

### Manuell nedlasting (macOS, Linux)

Last ned siste utgivelse fra
[GitHub Releases](https://github.com/neokapi/neokapi/releases):

- **macOS**: `kapi-desktop-X.Y.Z-macOS-arm64.dmg` (Apple Silicon)
- **Linux**: `kapi-desktop-X.Y.Z-linux-amd64.tar.gz` eller `kapi-desktop-X.Y.Z-linux-arm64.tar.gz`
