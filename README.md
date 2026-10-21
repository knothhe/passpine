# Passpine

兼容 pass 的本地密码管理器，包含 Go 原生客户端和浏览器扩展。支持密码生成、TOTP、Git 同步及浏览器填充，客户端与扩展共用解锁会话。

目前已验收 macOS Apple Silicon；Linux 已在 Arch Linux 上验证构建、安装和原生演示窗口启动；Windows 提供原生构建、NSIS 安装包、安全文件访问和浏览器 Host 注册。

## 构建与安装

需要 Make、Go 1.27.1+、Node.js 22.12+、pnpm 12.11.2；macOS 还需 Xcode Command Line Tools。仅从系统密钥环自动导入时需要 GnuPG；手动导入密钥文件和日常加解密均不需要。Git 同步使用系统 Git 和已有认证凭据。

在仓库根目录运行：

```sh
make                    # 构建客户端和 Chrome / Firefox 扩展
make client:install     # 安装客户端并注册浏览器连接
make browser:install    # 按本地配置安装扩展
make help               # 查看全部命令和参数
```

macOS 构建产物为 `dist/Passpine.app`，默认安装到 `~/Applications/Passpine.app`。应用使用本地签名，未配置 Developer ID 公证。

Linux 同样使用 `make build` / `make install`，客户端产物为 `dist/passpine` 和 `dist/passpine-host`，默认安装到 `~/.local/bin`，并添加应用菜单入口和图标。运行需要 GTK 3、WebKitGTK 4.1，托盘需要桌面支持 StatusNotifierItem。在 Arch Linux 上安装运行依赖：

```sh
sudo pacman -S --needed gtk3 webkit2gtk-4.1
```

完整检查脚本还需要 `zip` 和 `unzip`。浏览器扩展安装仍需配置安装目录，详见下方本地安装说明。

Windows（64 位）还需要 NSIS 3，用于生成安装包。使用管理员 PowerShell 安装全局依赖（需先安装 Chocolatey）：

```powershell
choco install golang nodejs-lts make nsis -y
npm.cmd install --global pnpm@12.11.2
```

重新打开终端后，在仓库根目录运行相同的 `make build` / `make install`，Make 自动选择 Windows PowerShell 或 macOS/Linux Bash 流程。Windows 构建生成 `dist/passpine.exe`、`dist/passpine-host.exe` 和 `dist/Passpine-<版本>-windows-amd64-setup.exe`；安装包默认安装到 `%LOCALAPPDATA%\Programs\Passpine`，创建开始菜单快捷方式和卸载入口，并通过当前用户注册表注册已安装的 Chrome、Edge、Brave、Firefox。卸载保留密码库及应用数据。安装包尚未配置 Authenticode 签名。

`make client:install CLIENT_INSTALL_DIR="C:\Apps\Passpine"` 可指定客户端安装目录。`make install` 还会按下方的本地配置安装浏览器扩展；仅安装客户端使用 `make client:install`。Windows 支持使用当前账户的登录密码进行系统解锁，首次使用私钥口令打开密码库后完成启用；修改 Windows 登录密码后需重新启用。

扩展安装前需配置安装目录，安装后需在浏览器中加载。详见[本地安装说明](docs/install-local.md)。

## 正式使用与原库迁移

1. 打开客户端，创建新库、打开本地 pass 密码库或下载 Git 仓库；默认识别 `~/.password-store`。新库口令至少 12 个字符。
2. 打开已有库时选择密钥来源：默认通过系统 GPG 按 `.gpg-id` 自动匹配，也可手动选择 OpenPGP 私钥文件及可选公钥文件，再输入私钥口令。支持 `.asc` / `.pgp` / `.gpg` / `.key`；私钥文件通常已含公钥，多收件人库需补齐对应公钥。暂不支持硬件卡私钥。
3. 导出恢复私钥并验证备份，妥善保存密码库目录、恢复私钥和口令。
4. 按需在设置中配置 Git 同步和浏览器连接。使用扩展时需保持客户端运行。

密码库保留 pass 的 `.gpg` / `.gpg-id` 格式，私钥保存在应用数据目录，不进入同步仓库。macOS 默认数据目录为 `~/Library/Application Support/Passpine/`。

旧 Go 客户端配置不会自动迁移，可用 `--data-dir <原数据目录>` 或 `PASSPINE_HOME` 继续使用；浏览器 Host 需设置相同的 `PASSPINE_HOME`。迁移到默认目录前先关闭客户端并备份，确认目标无其他数据后复制配置及 `keys`，再重新启用系统解锁。

## 开发与预览

```sh
pnpm run dev                                    # 运行客户端
pnpm run preview                                # 隔离的客户端演示
pnpm --dir browser install --frozen-lockfile
pnpm run preview:browser                         # 浏览器预览：http://127.0.0.1:5174/
bash scripts/check.sh                            # 运行检查与测试
```

演示使用虚构账号，不读取个人密码库；客户端演示口令为 `Passpine-demo-2026`。浏览器预览不连接真实客户端。

## 发版

提交所有修改后，在干净的 `main` 分支使用已登录的 GitHub CLI 执行：

```sh
make release-initial RELEASE_FLAGS=--apply    # 首次发布当前版本
make release RELEASE_FLAGS=--apply            # 后续升级 patch 并发布
```

省略 `RELEASE_FLAGS=--apply` 可预览。脚本自动运行检查、构建当前 Mac 架构客户端和 Chrome / Firefox 扩展、创建 annotated tag 并推送，上传三个成品 ZIP 后公开 GitHub Release。失败续传、附件和签名范围见[版本与发布](docs/versioning.md)。

## 文档

- [浏览器扩展开发](browser/README.md)
- [桌面设置与生命周期](docs/desktop-settings.md)
- [功能与验证记录](docs/VALIDATION.md)
- [架构与安全设计](docs/ARCHITECTURE.md)
- [版本与发布](docs/versioning.md)
