# 版本管理与发布

客户端和浏览器扩展共享一个稳定版本。根目录 `package.json` 是统一版本来源，`browser/package.json` 必须一致。客户端构建将版本注入客户端和 Native Messaging Host 的 `--version`、macOS `CFBundleShortVersionString` / `CFBundleVersion`；WXT 从扩展包读取 manifest 和 ZIP 版本。

## 一条命令发布

在仓库根目录执行，要求已提交所有修改、检出 `main`、工作区干净，并已使用 `gh auth login` 登录有仓库发布权限的 GitHub 账号。需要项目构建依赖和 macOS 自带的 `unzip`；当前完整附件发布支持在 macOS arm64 或 x86_64 上构建本机架构客户端，不生成其他平台或架构的客户端包。

```sh
# 首次发布当前的 0.1.0，不增加版本、不创建空的版本提交
pnpm run release 0.1.0 --apply

# 后续升级并发布，例如 0.1.0 → 0.1.1
pnpm run release patch --apply
pnpm run release minor --apply
pnpm run release 0.2.0 --apply

# 去掉 --apply 只预览；不执行测试、构建或远程写入
pnpm run release 0.1.0

# Make 等价入口，VERSION_BUMP 同样接受精确版本
make release VERSION_BUMP=0.1.0 RELEASE_FLAGS=--apply
```

`.release.json` 默认启用 `github_release` 和 `build_artifacts`。发布顺序为：

1. 检查工作区、分支、两端版本、远程分支/tag 冲突及 `gh` 登录状态。
2. 检查版本一致性并运行 `pnpm test`（发布工具、Swift、Go race 测试、Go vet、扩展类型检查和测试）。
3. 目标版本更高时同步写入两个 `package.json`；目标等于当前版本且尚无同名 tag 时直接发布当前版本。目标低于当前版本会被拒绝。
4. 通过 `make client:build` 和 `pnpm run package:browser` 构建目标版本，校验 ZIP 完整性、包内版本，并将成品复制到 `dist/releases/<版本>/`。
5. 升版时只提交两个版本文件，提交名为 `chore(release): v<版本>`；发布当前版本时保留原 HEAD。创建 `v<版本>` annotated tag，再以 atomic push 一次推送 `main` 和该 tag。
6. 创建 GitHub Release 草稿，上传三份指定附件，核对远程附件大小与 SHA-256，全部通过后公开 Release 并输出 URL。

每项检查和构建后都会核对 HEAD、分支、refs、index、版本文件和工作区；发现副作用立即停止并保留现场。忽略的构建输出不进入版本提交。构建发生在 tag 和推送之前，因此构建失败不会发布有缺失附件的新版本。本地尚未推送的分支提交也会随 `main` 一起推送。

预览仍要求工作区干净，默认只读查询远程 refs；不会检查 `gh` 登录、运行测试或构建，因此预览成功不代表执行一定成功。先审核并提交发版脚本和配置的修改，再使用正式发布命令。

## 发布附件

附件统一保存在 `dist/releases/<版本>/`，只上传下表中的确切文件，不使用 `*.zip` 通配符。

| 附件 | 内容 |
| --- | --- |
| `Passpine-<版本>-macos-arm64.zip` 或 `Passpine-<版本>-macos-x86_64.zip` | 当前 Mac 架构的 `Passpine.app`，包含客户端、Native Messaging Host 和系统解锁辅助程序 |
| `passpine-<版本>-chrome.zip` | Chrome / Edge 共用的 MV3 扩展 |
| `passpine-<版本>-firefox.zip` | Firefox MV3 扩展 |

客户端沿用本地 ad-hoc 签名，未接入 Developer ID 公证。扩展 ZIP 用于解包加载，不自动提交商店或获取 Firefox 签名；Firefox 临时加载在重启浏览器后需要重新加载。WXT 关闭额外 sources ZIP 生成；GitHub 自动提供的 Source code 下载项不受影响。

构建开始前删除本次预期的三个原始 ZIP，防止误用旧产物；打包后校验 macOS 两个 bundle 版本或扩展 manifest 版本。`dist/releases/<版本>/manifest.json` 记录发布 HEAD、文件名、大小和 SHA-256，用于失败后复用同一份文件续传；它仅保存在本地，不上传。

Release 正文保留 Full Changelog 链接，并列出上一可达较低 SemVer tag 之后的非 merge 提交标题与短 hash，排除版本提交。首次发布列出全部历史；需要完整历史与 tags。适配标准 github.com push URL，Enterprise / SSH alias 需要额外配置。没有接入 GitHub Actions 或 npm registry 发布。

草稿及附件行为依据 [GitHub CLI create](https://cli.github.com/manual/gh_release_create)、[upload](https://cli.github.com/manual/gh_release_upload)、[edit](https://cli.github.com/manual/gh_release_edit) 和 [Release assets API](https://docs.github.com/en/rest/releases/assets)。

## 版本与本地操作

采用 `MAJOR.MINOR.PATCH`：兼容修复升 patch，兼容功能升 minor，不兼容公共接口变更升 major。minor 清零 patch，major 清零后两位。`0.x` 仍处于未稳定阶段，不自动把所有变更判断为 major。

为兼容 Chrome / Edge，当前统一版本只接受稳定三段数字，每段为 0–65535，且不能为 `0.0.0`。不接受预发布、build metadata 或前导零。pnpm 锁文件不含项目自身版本，升版不会重新解析依赖。

```sh
make version:show
make version:check
make version:patch
make version:minor
make version:major
make version:set VERSION_VALUE=0.2.0

# 同样支持 pnpm run version:show / version:check / version:patch 等
pnpm run version:set 0.2.0

# 本地预览或执行：仍构建附件，但不推送、不创建 GitHub Release
pnpm run release 0.1.0 --no-push --no-github-release
pnpm run release 0.1.0 --apply --no-push --no-github-release
```

版本修改命令只更新两个 `package.json`，不提交、不打 tag、不构建或推送；修改前要求版本一致且目标更高。开发时 `go run` 的 `--version` 返回 `development`，正式构建才注入版本。`--no-github-release` 可覆盖默认配置关闭 GitHub Release；`--no-push` 必须同时关闭 GitHub Release。`build_artifacts: false` 可关闭项目附件构建和上传，仅保留版本/tag/Release 流程。

## 失败与恢复

默认测试在写版本前执行；失败时尚未修改版本。构建在写入目标版本后执行；升版构建失败时两个版本文件的修改会保留，尚无本次版本提交/tag。先检查并修复原因，只恢复本次脚本造成的版本修改，保持干净工作区后重跑原命令。不通过跳过 hook、强推、移动 tag 或自动 reset/clean 绕过错误。

若 annotated tag 已创建，但推送、附件上传或 Release 发布失败，确认当前 HEAD、版本及 tag 后，以精确版本续跑：

```sh
pnpm run release 0.1.0 --resume --apply
```

`--resume` 要求当前版本一致、同版本 annotated tag 指向 HEAD、远程 tag 不冲突；支持首次发布时直接打 tag 的普通提交。不升版、不重复测试，优先复用本地 manifest 中与 HEAD 一致且校验通过的附件；完全没有 manifest 时重新构建附件。manifest 不匹配或文件校验失败时停止，需先人工检查。

上传中断时 Release 保持草稿。续传会保留正文和已上传的匹配附件，仅补传缺失文件，最后公开 Release。远程同名附件必须大小、SHA-256 和上传状态都匹配，脚本不会覆盖或删除附件；已公开且附件完整的 Release 只返回 URL。已公开但缺失附件、同名附件内容冲突或 GitHub 查询失败时均停止，需人工核查；不要通过移动 tag 重发。缺少本地缓存后重新构建的 ZIP 可能因时间戳或签名不同而与已上传文件产生冲突，应妥善保留 `dist/releases/<版本>/`。
