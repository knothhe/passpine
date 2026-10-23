# 版本管理与发布

客户端和浏览器扩展在同次发布中共享一个稳定版本。根目录 `package.json` 是统一版本来源，`browser/package.json` 必须一致。客户端构建将版本注入客户端和 Native Messaging Host 的 `--version`、macOS bundle；WXT 从扩展包读取 manifest 和 ZIP 版本。发布版本号不参与连接校验：用户安装的客户端与扩展可以来自不同发布，只要通信协议兼容即可，见[通信兼容规则](ARCHITECTURE.md#协议兼容)。

## 一条命令发布

要求已提交所有修改、检出 `main`、工作区干净，并能推送远程仓库。首次发布还需要 `gh auth login`，用于确认 GitHub 上不存在同名 Release 或可见草稿。

```sh
# 首次发布当前版本，保留 HEAD，不创建版本提交
make release-initial RELEASE_FLAGS=--apply

# 后续升级并发布
make release RELEASE_FLAGS=--apply
make release VERSION_BUMP=minor RELEASE_FLAGS=--apply
make release VERSION_BUMP=0.1.4 RELEASE_FLAGS=--apply

# 预览，不执行测试或远程写入
make release-initial
make release
```

`make release` 默认预览 patch 升级，`VERSION_BUMP` 支持 `patch`、`minor`、`major` 或精确版本。`.release.json` 启用 `ci_release`，关闭本地 `github_release` 和 `build_artifacts`，由 Actions 负责构建和公开 Release。启用 CI 时不能通过 `--github-release` 另行发布。

1. 本地检查工作区、分支、两端版本及远程分支/tag 冲突，运行版本检查与 `pnpm test`。每项检查后核对 HEAD、分支、refs、index、版本文件和工作区，发现副作用立即停止。
2. 升版时同步两个 `package.json`，只提交版本文件，提交名为 `chore(release): v<版本>`；首次发布保留 HEAD，tag 正文带 `Release-Type: initial`。
3. 创建 `v<版本>` annotated tag，以 atomic push 一次推送 `main` 和该 tag。本地尚未推送的分支提交也会随 `main` 一起推送。
4. tag push 触发 `.github/workflows/release.yml`，复用 CI，在各目标系统运行检查并构建。完整检出历史与 tags，核对 tag、HEAD 及版本文件一致。
5. 收齐五份普通安装附件，校验正式通道、包内版本、构建提交、大小与 SHA-256，再验证普通安装包与各平台目标文件清单并签署 Feed。创建 Release 草稿，上传并核对五个普通安装附件和 Feed 共六个远程附件后公开 Release。任何构建失败都不会进入发布步骤。

本地命令完成表示 tag 已推送，最终发布结果以 Actions 的 Release 工作流为准。仓库需启用 GitHub Actions；发布 job 使用内置 `GITHUB_TOKEN` 的 `contents: write`，检查和构建 job 只有 `contents: read`。发布要求已固定的 Ed25519 公钥与对应 CI 签名私钥，详见 [客户端更新](updates.md)。

预览仍要求工作区干净，默认只读查询远程 refs；首次发布到标准 GitHub 目标还会查询已有 Release。预览不运行测试或验证写入权限。应先审核并提交工作流、脚本和配置，再正式发布。

## CI 与发布附件

`.github/workflows/ci.yml` 仅作为 Release 的可复用工作流，必须传入已有发布 tag；普通分支 push 和 pull request 不触发 GitHub CI。`.github/workflows/release.yml` 在推送 `v*` tag 时自动运行，也可手动输入已有 tag 重试。构建目标为 macOS arm64、Windows amd64 和 Linux amd64。macOS 使用系统 Swift 工具链；Windows 安装 NSIS；Linux 安装 GTK 3 和 WebKitGTK 4.1。浏览器打包集中在 Linux job。

| 附件 | 内容 |
| --- | --- |
| `Passpine-<版本>-macos-arm64.zip` | Apple Silicon 应用包，含客户端、Host 和系统解锁辅助程序 |
| `Passpine-<版本>-windows-amd64-setup.exe` | 当前用户安装程序，含客户端、Host、快捷方式及浏览器注册 |
| `Passpine-<版本>-linux-amd64.tar.gz` | 客户端、Host、图标、版本文件与 `install.sh` |
| `passpine-<版本>-chrome.zip` | Chrome / Edge 共用的 MV3 扩展 |
| `passpine-<版本>-firefox.zip` | Firefox MV3 扩展 |
| `update-feed.json` | 包地址、完整性清单及 Ed25519 签名；与普通安装包共用制品 |

Linux 下载包解压到单独目录，运行 `bash install.sh`，默认安装到 `~/.local/bin`，也可传入其他绝对安装目录。安装器创建桌面菜单入口并注册 Host；目标系统需要 GTK 3 和 WebKitGTK 4.1，无需 Node.js。CI 构建不会安装到用户目录。

macOS 沿用 ad-hoc 签名，未接入 Developer ID 公证；Windows 未接入 Authenticode 签名。扩展 ZIP 不自动提交商店或获取 Firefox 签名。WXT 关闭额外 sources ZIP 生成，GitHub 自动提供的 Source code 下载项不受影响。

各 job 将附件暂存到 `dist/releases/<版本>/`，生成独立的 `manifest-<目标>.json`，记录 tag、HEAD、版本、文件名、大小和 SHA-256。发布步骤要求完整且匹配的目标集合，只上传确切文件。manifest 保存在 Actions artifacts 中，不上传到 Release。

客户端 job 的来源 manifest 同时记录普通安装包哈希、更新格式和最终受控程序文件的大小、SHA-256 与权限，不再生成专用 update ZIP。publication job 使用 `PASSPINE_UPDATE_PRIVATE_KEY` Secret 签署 `update-feed.json`，与五个普通安装附件一起发布。签名缺失、固定公钥不匹配、兼容 Feed 验签失败或普通包与目标文件清单不一致时停止发布；私钥不会进入构建 job。

Feed 外层保留固定 v0.1.5 兼容信息，新的 `installerUpdate` 使用 schema 2 并直接引用普通安装包。旧版用户需手动安装一次支持此协议的新正式版，之后自动更新不再需要额外程序包。Windows 普通安装器提供仅解包协议，CI 在发布前验证其隔离及参数拒绝行为；publication job 使用 7-Zip 静态检查 NSIS 内容，不执行安装器。公钥和更新 URL 沿用 `config/update-signing.json`，无需更换生产 Secret。具体格式及数据保护见 [客户端更新](updates.md)。

正文复用本地脚本逻辑，保留 Full Changelog 链接，列出上一可达较低 SemVer tag 之后的非 merge 提交标题与短 hash，排除版本提交。首次发布列出全部历史，需要完整历史与 tags。也可独立生成：

```sh
node scripts/release-notes.mjs knothhe/passpine v0.1.1
```

只支持稳定版本 tag `v<MAJOR.MINOR.PATCH>`；不发布 npm registry 包。

## 版本与本地操作

兼容修复升 patch，兼容功能升 minor，不兼容公共接口变更升 major。minor 清零 patch，major 清零后两位。`0.x` 仍未稳定，不自动把所有变更判断为 major。

为兼容 Chrome / Edge，版本只接受稳定三段数字，每段 0–65535，不能为 `0.0.0`，不接受预发布、build metadata 或前导零。pnpm 锁文件不含项目自身版本，升版不会重新解析依赖。

```sh
make version:show
make version:check
make version:patch
make version:minor
make version:major
make version:set VERSION_VALUE=0.1.4

# 只在本地创建版本提交与 tag，不推送、不触发 CI
make release-initial RELEASE_FLAGS="--apply --no-push"
make release VERSION_BUMP=patch RELEASE_FLAGS="--apply --no-push"
```

版本命令只更新两个 `package.json`，不提交、不打 tag、不构建或推送。直接 `go run` 默认 Dev 通道，`--version` 返回 `development`；本地 Make 构建注入 `<基础版本>-dev.<提交>`。扩展统一使用数字 manifest 版本与 Passpine 名称。`--build-channel` 可检查客户端及 Host 的实际通道。`--no-push` 不查询远程，不能证明版本尚未发布。

## Dev 与正式版

`make build` / `make install` 默认构建 Dev 客户端和统一扩展。Dev 客户端使用 `Passpine Dev` 应用名、独立的应用标识、数据目录和 Native Messaging Host；不迁移正式数据，不扫描 `~/.password-store`，不启用正式更新。客户端产物位于 `dist/dev/`，扩展始终使用 `browser/.output/`。扩展不再按构建通道切换身份；商店安装固定连接正式客户端，本地解包加载可在设置中选择 Dev 客户端。具体安装位置见 [本地安装](install-local.md)。

正式客户端只由 GitHub Actions 在匹配版本的发布 tag 上设置 `PASSPINE_BUILD_CHANNEL=release` 后构建。脚本校验 Actions 环境、仓库、tag/HEAD 和源码状态，发布步骤再次检查制品身份。仅在本地设置 `PASSPINE_BUILD_CHANNEL=release` 不会生成正式客户端；客户端本地安装入口也拒绝正式通道，正式客户端请下载 Release 安装包。统一扩展可在本地构建与安装，无需设置客户端发布通道。

## 失败与恢复

本地测试在写版本前执行，失败时尚未修改版本。推送失败时，核对 HEAD、版本及 annotated tag 后，用精确版本续推：

```sh
make release VERSION_BUMP=0.1.1 RELEASE_FLAGS="--resume --apply"
```

`--resume` 要求当前版本一致、同版本 annotated tag 指向 HEAD、远程 tag 不冲突；首次发布通过 initial 标记识别，升版校验版本提交。不升版、不重复本地测试，不强推或移动 tag。旧首次发布 tag 没有 initial 标记时，需单独核查原发布，不修改 tag。

tag 已推送但 CI 失败时，在 Actions 中重跑失败 jobs。发布步骤失败时优先只重跑失败 job，复用成功构建 job 的原始附件；重建 ZIP 可能因时间戳或签名不同而与已上传文件冲突。也可从 Release 工作流的 Run workflow 输入已有 tag 重新执行完整流程，适用于尚未上传附件的失败发布。

上传中断时 Release 保持草稿。重试保留正文和已上传的匹配附件，只补传缺失文件。远程同名附件必须大小、SHA-256 和状态一致，脚本不会覆盖或删除附件。已公开且完整的 Release 只返回 URL；已公开但缺少附件、同名内容冲突或 GitHub 查询失败均停止，需人工核查，不通过修改 tag 重发。Actions artifacts 过期后，应先核查草稿和已有附件再重新构建。

相关行为依据 [GitHub 可复用工作流](https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows)、[GitHub CLI create](https://cli.github.com/manual/gh_release_create) 和 [Release assets API](https://docs.github.com/en/rest/releases/assets)。
