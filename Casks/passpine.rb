cask "passpine" do
  version "0.1.7"
  sha256 "1a896db9555d7d10180163ade4b59fb5ee93efc17cc0c174daaba38fb0b3c4c8"

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
