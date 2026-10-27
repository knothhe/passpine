# Passpine

[English](README.md) | 简体中文

兼容 [pass](https://www.passwordstore.org/) 的本地密码管理器，提供 macOS、Windows、Linux 原生桌面客户端和浏览器扩展，支持密码生成、TOTP、Git 同步及网页填充。

无需安装 GnuPG 即可创建 OpenPGP 加密密码库，也可通过系统 GPG 或私钥文件导入已有 pass 密码库。

## 安装

**macOS** — Apple Silicon，macOS 13 及以上：

```sh
brew install --cask knothhe/tap/passpine
```

**Linux** — x86_64：

```sh
curl -fsSL https://raw.githubusercontent.com/knothhe/passpine/main/scripts/install-linux.sh | sh
```

**Windows** — x86_64：从 [GitHub Releases](https://github.com/knothhe/passpine/releases) 下载安装包。该页面也提供 macOS、Linux 安装包及 Chrome / Firefox 扩展。

平台依赖、浏览器配置及卸载方式见[安装说明](docs/install-local.md)。macOS 应用尚未进行 Developer ID 公证，Windows 安装包尚无 Authenticode 签名。

## 开始使用

1. 打开客户端，创建新库、打开本地 pass 密码库或下载 Git 仓库。新建密码库需设置至少 12 个字符的口令。
2. 导出恢复私钥并验证备份，妥善保存密码库、恢复私钥和口令。
3. 按需在设置中启用 Git 同步或浏览器连接。使用扩展时需保持客户端运行。

macOS 和 Linux 还提供兼容 pass 的命令行，例如 `passpine show -c email/work`，详见[命令行说明](docs/cli.md)。

## 开发

需要 Make、Go 1.27.1+、Node.js 22.12+ 和 pnpm 12.11.2，各平台额外依赖见安装说明。在仓库根目录运行：

```sh
make                     # 构建 Dev 客户端和 Chrome / Firefox 扩展
make client:install      # 安装 Dev 客户端并注册浏览器连接
make help                # 查看命令和参数
pnpm run preview         # 运行客户端演示
pnpm --dir browser install --frozen-lockfile
pnpm run preview:browser # 浏览器演示：http://127.0.0.1:5174/
pnpm test                # 运行检查与测试
```

Dev 客户端使用独立的应用和数据目录。本地加载的扩展默认连接正式客户端，可在扩展设置中切换到 Dev。两种演示均使用虚构数据，客户端演示口令为 `Passpine-demo-2026`。

## 文档

- [安装与浏览器配置](docs/install-local.md) · [命令行](docs/cli.md)
- [浏览器扩展](browser/README.md) · [官网](website/README.md)
- [桌面设置](docs/desktop-settings.md) · [R2 / S3 配置同步](docs/config-sync.md)
- [架构与安全](docs/ARCHITECTURE.md) · [自动更新](docs/updates.md)
- [版本与发布](docs/versioning.md) · [验证记录](docs/VALIDATION.md)

## 开源协议

[MIT](LICENSE)
