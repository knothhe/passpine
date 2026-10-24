cask "passpine" do
  version "0.1.10"
  sha256 "2baa5890d1c3b085501062d2c862a26797dd8905b28003b21e215122aff2e965"

  url "https://github.com/knothhe/passpine/releases/download/v#{version}/Passpine-#{version}-macos-arm64.zip"
  name "Passpine"
  desc "Local password manager compatible with pass"
  homepage "https://github.com/knothhe/passpine"

  livecheck do
    url :url
    strategy :github_latest
  end

  auto_updates true
  depends_on arch: :arm64
  depends_on macos: :ventura

  app "Passpine.app"
  binary "#{appdir}/Passpine.app/Contents/MacOS/passpine"

  uninstall quit: "com.knothhe.passpine"
end
