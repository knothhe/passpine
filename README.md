# Passpine

兼容 [pass](https://www.passwordstore.org/) 的本地密码管理器，包含桌面客户端和浏览器扩展，支持密码生成、TOTP、Git 同步及网页填充。

无需用过 pass 或安装 GnuPG：创建密码库时，Passpine 会自动生成受口令保护的 OpenPGP 私钥。已有 pass 密码库也可通过系统 GPG 或密钥文件导入。

## macOS 安装

Apple Silicon Mac（macOS 13 及以上）可通过 Homebrew 安装正式客户端：

```sh
brew install --cask knothhe/tap/passpine
```

命令会自动添加 [knothhe/tap](https://github.com/knothhe/homebrew-tap)。也可从 [GitHub Releases](https://github.com/knothhe/passpine/releases) 下载。三平台构建及正式版发布成功后，GitHub Actions 会自动更新 Homebrew 配方。Homebrew 更新、卸载及首次启动说明见[安装说明](docs/install-local.md#macos-homebrew-正式版)。

## Linux 安装

x86_64 Linux 可一条命令安装正式客户端：

```sh
curl -fsSL https://raw.githubusercontent.com/knothhe/passpine/main/scripts/install-linux.sh | sh
```

脚本识别 Arch、Debian/Ubuntu、Fedora/RHEL、openSUSE 及其衍生版，用系统包管理器补装缺少的 GTK 3、Git 和 OpenSSL（仅此步骤使用 `sudo` 或 `doas`）；随后用固定公钥验证 Release 的签名更新 Feed，核对安装包及程序的 SHA-256，再运行包内 `install.sh` 安装到 `~/.local/bin`、创建桌面入口并注册浏览器连接。之后的升级由应用内更新完成。

在 `sh -s --` 后传入参数：`--version X.Y.Z` 安装指定版本，`--skip-deps` 跳过依赖安装，`--uninstall` 注销浏览器连接并移除程序、桌面入口和图标，保留密码库、私钥与设置。`PASSPINE_BIN_DIR` 可改安装目录。脚本只替换或移除正式版 Passpine，不覆盖其他程序、符号链接或指向其他目录的桌面入口。

## 构建与安装

需要 Make、Go 1.27.1+、Node.js 22.12+ 和 pnpm 12.11.2；macOS 还需 Xcode Command Line Tools。Git 同步使用系统 Git 和已有认证凭据。

在仓库根目录运行：

```sh
make                    # 构建 Dev 客户端和 Chrome / Firefox 扩展
make client:install     # 安装 Dev 客户端并注册独立的浏览器连接
make browser:install    # 按本地配置构建并安装统一扩展
make client:uninstall   # 卸载 Dev 客户端及自有注册项，保留数据
make help               # 查看全部命令和参数
```

本地 `make build` / `make install` 构建 Dev 客户端和统一浏览器扩展；正式客户端由 GitHub Actions 在发布 tag 上构建并发布，从 [GitHub Releases](https://github.com/knothhe/passpine/releases) 下载。Dev 客户端使用独立应用名称、应用标识、数据目录和 Native Host，不自动更新到正式版。扩展统一使用 Passpine 名称和同一扩展 ID，默认连接正式客户端；本地解包加载时可在设置中切换 Dev 客户端，商店安装只连接正式客户端。扩展安装前需配置安装目录，安装后需在浏览器中加载。平台依赖、安装位置及操作步骤见[本地安装说明](docs/install-local.md)。

Dev 客户端图标带右下角 `Dev` 标记。设置 → 关于提供通道、源码提交、未提交修改状态、构建环境与平台信息，可一键复制。退出会先处理未保存编辑，等待设置及正在执行的请求完成。

支持 macOS、Windows 和 Linux。macOS 正式版使用固定的自签名证书，更新后应用身份保持不变，但尚未配置 Developer ID 公证；Windows 安装包尚未配置 Authenticode 签名。

## 使用

1. 打开客户端，选择创建新库、打开本地 pass 密码库或下载 Git 仓库；正式版默认识别 `~/.password-store`，Dev 不自动发现该目录。
2. 创建新库时设置至少 12 个字符的口令，客户端自动创建私钥并初始化密码库。
3. 导出恢复私钥并验证备份，妥善保存密码库目录、恢复私钥和口令。
4. 按需在设置中启用 Git 同步和浏览器连接。使用扩展时需保持客户端运行。

客户端也提供兼容 pass 的命令行，如 `passpine show -c email/work`；可选运行 `passpine cli install --pass-alias` 添加 `pass` 别名。详见[命令行说明](docs/cli.md)。

打开已有库时，默认通过系统 GPG 按 `.gpg-id` 匹配密钥（仅此方式需要 GnuPG）；也可选择 `.asc` / `.pgp` / `.gpg` / `.key` 私钥文件并输入口令。私钥文件通常已含公钥，多收件人库需补齐对应公钥；暂不支持硬件卡私钥。

密码库保留 pass 的 `.gpg` / `.gpg-id` 格式，私钥保存在应用数据目录，不进入同步仓库。macOS 正式版数据目录为 `~/Library/Application Support/Passpine/`，Dev 为 `~/Library/Application Support/Passpine Dev/`。

## 开发与预览

```sh
pnpm run dev                                    # 运行客户端
pnpm run preview                                # 隔离的客户端演示
pnpm --dir browser install --frozen-lockfile
pnpm run preview:browser                         # 浏览器预览：http://127.0.0.1:5174/
bash scripts/check.sh                            # 运行检查与测试
```

演示使用虚构账号，不读取个人密码库；客户端演示口令为 `Passpine-demo-2026`。浏览器预览不连接真实客户端。

## 文档

- [本地安装与平台依赖](docs/install-local.md)
- [命令行与 pass 别名](docs/cli.md)
- [浏览器扩展开发](browser/README.md)
- [官网开发与部署](website/README.md)
- [Chrome Web Store 提交材料](docs/chrome-web-store/README.md)
- [桌面设置与生命周期](docs/desktop-settings.md)
- [R2 / S3 配置同步](docs/config-sync.md)
- [客户端自动更新](docs/updates.md)
- [架构与安全设计](docs/ARCHITECTURE.md)
- [版本与发布](docs/versioning.md)
- [功能与验证记录](docs/VALIDATION.md)

## 开源协议

本项目采用 [MIT License](LICENSE)。
