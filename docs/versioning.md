# 版本管理与发布

本项目按 `h2-semver-release` skill 接入 Node.js 发布流程，并适配客户端与浏览器两个版本源。根目录 `package.json` 是统一版本来源；`browser/package.json` 必须一致。构建脚本从根目录读取版本，写入客户端与 Native Messaging Host 的 `--version`、macOS `CFBundleShortVersionString` / `CFBundleVersion` 和 `Passpine-<版本>-macos-<架构>.zip`。WXT 从扩展包读取 manifest 和 ZIP 版本。

## 选择版本

采用 `MAJOR.MINOR.PATCH`：兼容修复升 patch，兼容功能升 minor，不兼容公共接口变更升 major。minor 清零 patch，major 清零后两位。`0.x` 仍处于未稳定阶段，不自动把所有变更判断为 major。

Chrome / Edge manifest 对版本有额外限制，因此当前统一发布只接受稳定三段数字版本，每段为 0–65535，且不能为 `0.0.0`。不接受预发布、build metadata 或前导零；需要此类发布时，应先设计独立的 manifest 版本映射。pnpm 独立包锁文件不含项目自身版本，版本变更不会重新解析依赖。

## 只更新本地版本

```sh
make version:show
make version:check
make version:patch
make version:minor
make version:major
make version:set VERSION_VALUE=0.2.0

pnpm run version:show
pnpm run version:check
pnpm run version:patch
pnpm run version:minor
pnpm run version:major
pnpm run version:set 0.2.0
```

修改命令先检查两个源一致且目标版本更高，再同步更新。它们只改两个 `package.json`，不提交、不创建 tag、不推送。检查报错时先人工核对版本来源。开发时直接 `go run` 的 `--version` 返回 `development`；正式版本由 `make client:build` 注入。

## h2 发布流程

`.release.json` 默认使用 `main` 分支、`origin`、`v` tag 前缀，先检查版本一致性，再运行 `pnpm test`。GitHub Release 默认关闭，不接入 GitHub Actions、商店或 npm registry 发布，也不自动打包 / 上传附件。

```sh
make release                                      # 预览 patch
make release VERSION_BUMP=minor                   # 预览 minor
pnpm run release patch --no-push                  # 只预览本地发布
pnpm run release 0.2.0                            # 预览精确版本

# 真正执行会提交版本、创建 annotated tag 并推送
pnpm run release minor --apply
# 只在本地创建版本提交和 tag
pnpm run release patch --apply --no-push
# 可选，需 gh 已登录；创建 GitHub Release
pnpm run release patch --apply --github-release
```

没有 `--apply` 时只输出计划；不改版本、不执行检查、不创建提交 / tag。默认预览会只读查询远程 refs；`--no-push` 可预览纯本地计划。首次接入应先审核并提交脚本、配置和说明，再运行发布，当前配置工作不会自动执行真实发布。

执行要求工作区（含未跟踪文件）干净，版本文件已跟踪且当前检出 `main`。远程分支必须存在并是本地 HEAD 的祖先；本地尚未推送的提交也会一并推送。每项检查后核对 HEAD、分支、refs、index 和工作区，发现副作用即停止并保留现场。通过检查后只提交版本文件，创建 `chore(release): v<版本>` 提交及 annotated tag，以 atomic push 同时推送指定分支和 tag。不会绕过 hooks、强制推送、移动 tag 或自动回滚。

启用 GitHub Release 时，正文保留 Full Changelog 链接，并列出上一可达较低 SemVer tag 之后的非 merge 提交标题与短 hash，排除版本提交；要求完整历史和 tags。适配范围为标准 github.com push URL；Enterprise / SSH alias 需要额外配置。

## 失败与恢复

默认检查在写版本之前执行，失败时脚本尚未改版本。检查自身留下的改动会保留；先审核修复，恢复干净工作区，再重跑。提交或 hook 失败后检查 index、文件及是否已有版本提交；不要跳过 hook 或用 reset / clean 丢弃其他改动。

若版本提交和 tag 已完成但推送或 GitHub Release 失败，核对当前版本、annotated tag 和 HEAD，再使用精确版本续跑：

```sh
pnpm run release 0.2.0 --resume --apply
pnpm run release 0.2.0 --resume --apply --github-release
```

`--resume` 不再升级版本或重复检查，要求同版本 annotated tag 指向预期版本提交的 HEAD，远程 tag 不得冲突。若已有后续提交或远程变化，先人工核查，不移动原 tag。
