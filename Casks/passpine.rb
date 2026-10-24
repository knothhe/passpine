cask "passpine" do
  version "0.1.9"
  sha256 "daf68b04c3d5fe6dd1b487c46dba5f654034f4bb9a724b8e1c7b8b7f64f0ca84"

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
