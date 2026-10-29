cask "passpine" do
  version "0.1.15"
  sha256 "13fff4373d86f1c5e02347c852316363d2c5cfcbc14e137f22451c980710acd8"

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
