# Homebrew Cask formula for Kapi.
# Copy this file to homebrew-tap/Casks/kapi-desktop.rb after release.

require "#{Tap.fetch("neokapi", "tap").path}/lib/private_download_strategy"

cask "kapi-desktop" do
  version "0.1.0"
  sha256 :no_check # update after first release

  url "https://github.com/neokapi/neokapi/releases/download/v#{version}/KapiDesktop-v#{version}-arm64.dmg",
      using: GitHubPrivateRepositoryReleaseDownloadStrategy
  name "Kapi"
  desc "Desktop workbench for a project's content context"
  homepage "https://github.com/neokapi/neokapi"

  depends_on formula: "neokapi/tap/kapi-cli"

  app "Kapi.app"

  caveats <<~EOS
    The kapi CLI is provided by the kapi-cli formula (installed automatically).

    Double-click Kapi projects to open them in Kapi.

    Getting started:
      kapi-desktop    # launch the app
      kapi --help     # CLI reference
  EOS

  zap trash: [
    "~/.config/kapi-desktop",
    "~/Library/Application Support/kapi-desktop",
    "~/Library/Caches/io.github.neokapi.kapi-desktop",
    "~/Library/Preferences/io.github.neokapi.kapi-desktop.plist",
    "~/Library/WebKit/io.github.neokapi.kapi-desktop",
  ]
end
