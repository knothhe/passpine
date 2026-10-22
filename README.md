# Passpine

兼容 [pass](https://www.passwordstore.org/) 的本地密码管理器，包含桌面客户端和浏览器扩展，支持密码生成、TOTP、Git 同步及网页填充。

无需用过 pass 或安装 GnuPG：创建密码库时，Passpine 会自动生成受口令保护的 OpenPGP 私钥。已有 pass 密码库也可通过系统 GPG 或密钥文件导入。

## 构建与安装

需要 Make、Go 1.27.1+、Node.js 22.12+ 和 pnpm 12.11.2；macOS 还需 Xcode Command Line Tools。Git 同步使用系统 Git 和已有认证凭据。

在仓库根目录运行：

```sh
make                    # 构建客户端和 Chrome / Firefox 扩展
make client:install     # 安装客户端并注册浏览器连接
make browser:install    # 按本地配置构建并安装扩展
make help               # 查看全部命令和参数
```

扩展安装前需配置安装目录，安装后需在浏览器中加载。平台依赖、安装位置及操作步骤见[本地安装说明](docs/install-local.md)。

支持 macOS、Windows 和 Linux。macOS 应用使用本地签名，尚未配置 Developer ID 公证；Windows 安装包尚未配置 Authenticode 签名。

## 使用

1. 打开客户端，选择创建新库、打开本地 pass 密码库或下载 Git 仓库；默认识别 `~/.password-store`。
2. 创建新库时设置至少 12 个字符的口令，客户端自动创建私钥并初始化密码库。
3. 导出恢复私钥并验证备份，妥善保存密码库目录、恢复私钥和口令。
4. 按需在设置中启用 Git 同步和浏览器连接。使用扩展时需保持客户端运行。

打开已有库时，默认通过系统 GPG 按 `.gpg-id` 匹配密钥（仅此方式需要 GnuPG）；也可选择 `.asc` / `.pgp` / `.gpg` / `.key` 私钥文件并输入口令。私钥文件通常已含公钥，多收件人库需补齐对应公钥；暂不支持硬件卡私钥。

密码库保留 pass 的 `.gpg` / `.gpg-id` 格式，私钥保存在应用数据目录，不进入同步仓库。macOS 默认数据目录为 `~/Library/Application Support/Passpine/`。

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
- [浏览器扩展开发](browser/README.md)
- [Chrome Web Store 提交材料](docs/chrome-web-store/README.md)
- [桌面设置与生命周期](docs/desktop-settings.md)
- [架构与安全设计](docs/ARCHITECTURE.md)
- [版本与发布](docs/versioning.md)
- [功能与验证记录](docs/VALIDATION.md)
