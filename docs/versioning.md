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
6. Release 成功公开后，独立的 `homebrew` job 调用 `.github/workflows/homebrew.yml`，读取本次发布 tag 的公开 macOS 安装包，再次校验大小与 SHA-256，更新 `knothhe/homebrew-tap` 的 `Casks/passpine.rb`，并同步本仓库中的旧 tap 兼容配方。两处均以 `github-actions[bot]` 身份提交到 `main`。没有变化不创建提交，重跑旧 tag 不降级已有 cask；无需手动执行 `make homebrew:sync`。

本地命令完成表示 tag 已推送，最终发布结果以 Actions 的 Release 工作流为准。仓库需启用 GitHub Actions；`publish` 使用内置 `GITHUB_TOKEN` 的 `contents: write`，`homebrew` 的内置 token 只更新本仓库兼容配方，检查和构建 job 只有 `contents: read`。跨仓库推送使用专门的 `HOMEBREW_TAP_SSH_KEY`，只在 Homebrew job 中提供给 tap checkout；无需 PAT。更新签名私钥仍只提供给 `publish`。发布要求已固定的 Ed25519 公钥与对应 CI 签名私钥，详见 [客户端更新](updates.md)。

预览仍要求工作区干净，默认只读查询远程 refs；首次发布到标准 GitHub 目标还会查询已有 Release。预览不运行测试或验证写入权限。应先审核并提交工作流、脚本和配置，再正式发布。

## CI 与发布附件

`.github/workflows/ci.yml` 仅作为 Release 的可复用工作流，必须传入已有发布 tag；普通分支 push 和 pull request 不触发 GitHub CI。`.github/workflows/release.yml` 在推送 `v*` tag 时自动运行，也可手动输入已有 tag 重试。构建目标为 macOS arm64、Windows amd64 和 Linux amd64。macOS 使用系统 Swift 工具链；Windows 安装 NSIS；Linux 安装 GTK 3。浏览器打包集中在 Linux job。

发版前的 `pnpm test` 在 macOS 和 Linux 上都会执行隔离的 Linux 安装脚本回归，使用临时程序、模拟平台和桌面数据库命令，检查脚本依赖、Dev 图标、安装归属记录及特殊路径转义。CLI 编辑回归使用原生 Go 测试进程，不依赖 `sed`、`true` 或 MSYS 文件重命名；Windows 单独验证命令链接操作被明确拒绝且不修改现有文件。模拟与交叉编译不能代替目标系统运行，Release 矩阵仍在各平台执行完整测试。

Linux 矩阵还在 Xvfb 中运行原生启动回归，确认 Dev 卸载控制端点在原生就绪回调之后才可见，再验证实际安装、退出、重复卸载和数据保留。原生验证失败时输出隔离测试进程的日志与 Go 堆栈，以便定位退出阻塞。

| 附件 | 内容 |
| --- | --- |
| `Passpine-<版本>-macos-arm64.zip` | Apple Silicon 应用包，含客户端、Host 和系统解锁辅助程序 |
| `Passpine-<版本>-windows-amd64-setup.exe` | 当前用户安装程序，含客户端、Host、快捷方式及浏览器注册 |
| `Passpine-<版本>-linux-amd64.tar.gz` | 客户端、Host、图标、版本文件与 `install.sh` |
| `passpine-<版本>-chrome.zip` | Chrome / Edge 共用的 MV3 扩展 |
| `passpine-<版本>-firefox.zip` | Firefox MV3 扩展 |
| `update-feed.json` | 包地址、完整性清单及 Ed25519 签名；与普通安装包共用制品 |

Linux 可用 `scripts/install-linux.sh` 一键安装（命令见 README）：它从最新或指定版本的 Release 下载 `update-feed.json`，以 `config/update-signing.json` 中的同一固定公钥验签，按 Feed 中的地址、大小和 SHA-256 核对安装包及两个程序，再执行包内 `install.sh`，因此不新增 Release 附件。脚本从 `main` 分支提供，须兼容所有含 `installerUpdate` 的已发布 Feed；修改 Feed 结构或 Linux 包内容时同步更新脚本及 `scripts/install-linux.test.mjs`。也可将下载包解压到单独目录，运行 `bash install.sh`，默认安装到 `~/.local/bin`，也可传入其他绝对安装目录。安装器创建桌面菜单入口并注册 Host；目标系统需要 GTK 3。客户端使用 MyGo 原生 UI，无需 WebKitGTK 或 Node.js。CI 构建不会安装到用户目录。

macOS 正式版使用固定的自签名证书签名，仍未接入 Developer ID 公证；Windows 未接入 Authenticode 签名。扩展 ZIP 不自动提交商店或获取 Firefox 签名。Chrome ZIP 使用独立商店配置，manifest 必须省略 `key`；本地解包构建保留公钥以固定 ID。发布暂存及汇总均直接检查 ZIP，拒绝任何包含 `key` 字段的 Chrome 包；Firefox 继续校验固定公钥及 Gecko ID。WXT 关闭额外 sources ZIP 生成，GitHub 自动提供的 Source code 下载项不受影响。

### macOS 签名身份

ad-hoc 签名的指定要求（designated requirement）是每次构建都会变化的 cdhash，更新后系统会把新版本视为另一个应用：通过 `SMAppService` 注册的开机自启及其他绑定代码身份的系统授权可能失效或需要重新批准。正式版因此改用长期固定的自签名证书，指定要求为 `identifier "com.knothhe.passpine" and certificate leaf = H"…"`，跨版本保持不变。它不能代替 Developer ID 公证：从浏览器下载、带 quarantine 属性的应用仍会被 Gatekeeper 拦截。本地 Dev 构建继续使用 ad-hoc 签名，也可设置 `PASSPINE_MACOS_SIGNING_IDENTITY`（及可选的 `PASSPINE_MACOS_SIGNING_KEYCHAIN`）改用本机身份。

证书 SHA-256 固定在 `config/macos-signing.json`。macOS 构建步骤运行 `node scripts/macos-signing.mjs build`：把 Secret 中的身份导入 `RUNNER_TEMP` 下的临时钥匙串，仅向该步骤的 `build-client.sh` 提供身份，结束后恢复钥匙串搜索列表并删除临时钥匙串。正式构建签名后检查 bundle 与 `Contents/MacOS` 中每个程序的叶证书均为固定证书，且指定要求基于证书而非 cdhash；缺少 Secret、证书不匹配或配置缺失时停止构建，不回退 ad-hoc。

| GitHub 配置 | 格式及用途 |
| --- | --- |
| Repository secret `PASSPINE_MACOS_SIGNING_CERTIFICATE` | Base64 编码的 PKCS#12 身份（证书与私钥），只传给 Release 的 build job 中的 macOS 构建步骤 |
| Repository secret `PASSPINE_MACOS_SIGNING_CERTIFICATE_PASSWORD` | 上述 PKCS#12 的口令 |

首次配置时在仓库外执行 `node scripts/macos-signing.mjs generate`，它在 `~/.config/passpine-release/macos-signing/`（目录 `0700`、文件 `0600`）生成证书、PKCS#12 和口令，写入 `config/macos-signing.json` 并打印设置 Secret 的命令；应另行保管离线备份。已有身份或仓库已固定证书时命令拒绝覆盖；确需更换时运行 `generate --force`，旧的证书、PKCS#12 和口令移到输出目录下的 `replaced-<时间>/` 而不删除，再按提示重新设置两个 Secret 并提交新的 `config/macos-signing.json`。更换证书会再次改变所有已安装客户端的应用身份，不要为修复 CI 配置重新生成。从 ad-hoc 版本升级到首个固定证书版本时，应用身份会变化一次，此后保持稳定。

也可以在 macOS 上运行 `node scripts/macos-signing.mjs upload [--repo OWNER/NAME]`，从钥匙串搜索列表中已有的代码签名身份（含未受信任的自签名证书）选择一个上传，默认仓库为 `knothhe/passpine`。命令要求 `gh` 已登录；由于 `security export` 只能导出整个钥匙串的身份，命令在私有临时目录中导出后只重新打包所选证书及其私钥，生成新的随机口令，确认后用 `gh secret set` 写入上述两个 Secret，结束时删除临时文件。所选证书与 `config/macos-signing.json` 不同时，命令提示身份会变化并在上传后更新固定值，需提交该文件后再发布。

各 job 将附件暂存到 `dist/releases/<版本>/`，生成独立的 `manifest-<目标>.json`，记录 tag、HEAD、版本、文件名、大小和 SHA-256。发布步骤要求完整且匹配的目标集合，只上传确切文件。manifest 保存在 Actions artifacts 中，不上传到 Release。

客户端 job 的来源 manifest 同时记录普通安装包哈希、更新格式和最终受控程序文件的大小、SHA-256 与权限，不再生成专用 update ZIP。publication job 使用 `PASSPINE_UPDATE_PRIVATE_KEY` Secret 签署 `update-feed.json`，与五个普通安装附件一起发布。签名缺失、固定公钥不匹配、兼容 Feed 验签失败或普通包与目标文件清单不一致时停止发布；私钥不会进入构建 job。

Feed 外层保留固定 v0.1.5 兼容信息，新的 `installerUpdate` 使用 schema 2 并直接引用普通安装包。旧版用户需手动安装一次支持此协议的新正式版，之后自动更新不再需要额外程序包。Windows 普通安装器提供仅解包协议，CI 在发布前验证其隔离及参数拒绝行为；publication job 使用 7-Zip 静态检查 NSIS 内容，不执行安装器。公钥和更新 URL 沿用 `config/update-signing.json`，无需更换生产 Secret。具体格式及数据保护见 [客户端更新](updates.md)。

正文复用本地脚本逻辑，保留 Full Changelog 链接，列出上一可达较低 SemVer tag 之后的非 merge 提交标题与短 hash，排除版本提交。首次发布列出全部历史，需要完整历史与 tags。也可独立生成：

```sh
node scripts/release-notes.mjs knothhe/passpine v0.1.1
```

只支持稳定版本 tag `v<MAJOR.MINOR.PATCH>`；不发布 npm registry 包。

## 版本与本地操作

### 同步 Homebrew cask

[knothhe/homebrew-tap](https://github.com/knothhe/homebrew-tap) 的 `Casks/passpine.rb` 提供 macOS 正式客户端，使用 `brew install --cask knothhe/tap/passpine` 即可自动添加 tap 并安装。本仓库保留相同配方，兼容旧的 `knothhe/passpine` 安装命令。cask 版本独立于待发布的源码版本，始终指向已公开的安装包。

日常发布由 Release workflow 自动更新 cask，不需要额外命令。三平台检查、构建及发布成功后，`homebrew` job 才运行，检出两个仓库的 `main`，按本次确切 tag 查询公开稳定版，而不是查询可能已变化的 latest。各版本与手动触发的 cask job 共用串行队列，旧版本即使后完成也不会覆盖较新版本。普通分支 push/PR 不触发应用 CI 或配方同步。

脚本核对 Apple Silicon 包的名称、地址、上传状态、大小及 GitHub SHA-256，再下载并验证内容后原子更新 cask。每个目标仓库都先检查工作区干净、快进到远程 `main`，只暂存并提交 `Casks/passpine.rb`；已有 Formula、文档和其他配方保持原样。普通 push 到 `main`，不创建或移动 tag、不修改 Release、不强推。相同版本出现不同安装包哈希时停止，防止静默替换既有配方。自动提交只更新 tap，用户下次 `brew update` 即可获取新版本。

跨仓库认证使用 `knothhe/homebrew-tap` 的专用可写 SSH deploy key，私钥保存为 `knothhe/passpine` 的 Actions Secret `HOMEBREW_TAP_SSH_KEY`。此 key 只授权该 tap 仓库；`GITHUB_TOKEN` 仍仅对 Passpine 仓库有效。更换认证时，在 tap 的 Settings → Deploy keys 登记公钥并允许写入，将对应私钥存入上述 Secret；不把私钥提交到仓库或传给应用构建与发布签名步骤。

已公开版本需要补同步时，可在 Actions 的 **Sync Homebrew** 工作流中输入确切 tag，或执行：

```sh
gh workflow run homebrew.yml --repo knothhe/passpine --ref main -f tag=v0.1.10
```

此入口复用相同的校验、队列及提交脚本，不构建客户端、不升版、不重发 Release。

保留以下本地维护入口，供检查或修复配方使用；它不是发版的必需步骤：

```sh
make homebrew:sync
```

本地维护需要已登录的 GitHub CLI（`gh auth login`），默认查询最新公开稳定版，只更新本地配方，不提交、不推送或安装应用。查询、下载或验证失败时保留原文件；旧版本不会覆盖较新 cask。不需要在升源码版本时提前修改 cask。

### 源码版本

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

tag 已推送但 CI 因临时网络或 runner 故障失败、无需修改源码时，在 Actions 中重跑失败 jobs。发布步骤失败时优先只重跑失败 job，复用成功构建 job 的原始附件；重建 ZIP 可能因时间戳或签名不同而与已上传文件冲突。也可从 Release 工作流的 Run workflow 输入已有 tag 重新执行完整流程，适用于尚未上传附件的失败发布。

若失败原因需要修改源码或测试，先提交修复并通过本地检查，再按正常流程发布新的 patch 版本。已有 tag 始终指向原提交；仅修改 `main` 后重跑旧 tag，仍会运行旧代码。不要移动 tag，也不要将新提交的产物混入旧版本。

如果只有 `homebrew` job 失败，Release 已经公开，安装包不受影响。旧运行结束后，可重跑该失败 job，或从当前 `main` 手动运行 **Sync Homebrew** 并指定原 tag，补齐配方提交，无需重建或再次发布。主 tap 已更新而兼容 tap 失败时，重试会跳过相同配方并继续补齐。并发维护者提交导致非快进 push 被拒绝时同样处理；脚本不会强推覆盖远程修改。若启用任一目标仓库的 `main` 分支保护或仓库规则，需允许相应认证更新 cask，否则 job 会如实失败，不自动放宽规则。

上传中断时 Release 保持草稿。重试保留正文和已上传的匹配附件，只补传缺失文件。远程同名附件必须大小、SHA-256 和状态一致，脚本不会覆盖或删除附件。已公开且完整的 Release 只返回 URL；已公开但缺少附件、同名内容冲突或 GitHub 查询失败均停止，需人工核查，不通过修改 tag 重发。Actions artifacts 过期后，应先核查草稿和已有附件再重新构建。

相关行为依据 [GitHub 可复用工作流](https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows)、[GitHub Actions 并发队列](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/control-workflow-concurrency)、[GitHub CLI create](https://cli.github.com/manual/gh_release_create) 和 [Release assets API](https://docs.github.com/en/rest/releases/assets)。
