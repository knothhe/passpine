cask "passpine" do
  version "0.1.12"
  sha256 "d41a94777c2a8293732b03ca85699cbcd8f5afa15d8b2f1109e145055ebc98f2"

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
