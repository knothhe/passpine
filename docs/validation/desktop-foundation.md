# 桌面基础能力验证

2026-10-24，macOS / Apple Silicon arm64。所有新增交互检查使用临时虚构数据。

## 已执行

- `bash scripts/check.sh`：版本与依赖检查、工具脚本测试、Go 全包 race / vet、Swift 设备封装测试、浏览器类型检查和 291 项测试通过。工具脚本中依赖本机 `makensis`/`7z` 的 6 项静态安装包测试因工具缺失跳过；不能视为通过。
- `make browser:build`：Chrome / Firefox MV3 构建及 CSP 校验通过。
- `make client:build`：生成 `dist/dev/Passpine Dev.app`，本地 ad-hoc 签名校验通过；普通 ZIP 内使用 Dev 图标。
- `go test -race ./nativehost ./desktop ./app ./buildinfo ./assets`：覆盖退出等待、保存失败、取消草稿退出、超时与重试、其他安装拒绝、真实 stdio 辅助进程退出、构建信息复制与图标通道选择。真实辅助进程用 `PASSPINE_TEST_HOST` 指向临时编译产物；未设置时该用例会明确跳过。
- `node --test scripts/client-installation.test.mjs scripts/build-channel.test.mjs`：安装归属、错误通道、路径与数据重叠、符号链接、注册项归属、部分失败重试和数据保留通过。
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...` 与对应 `GOOS=linux`：代码交叉编译检查；不能代替目标系统运行。

原生界面使用从 Dev 构建复制的临时 `.app`，仅将副本 bundle ID 改为随机验证标识并重新本地签名，以 `--preview --data-dir <临时目录>` 启动。确认了英文浅色关于页、中文深色及 120% 字号、构建信息复制成功反馈；新建临时条目后退出会提示未保存，继续编辑保留输入，确认放弃后原进程以 0 退出，独立控制端点移除，语言/主题/字号已持久化。没有在现有安装上执行安装或卸载，也没有测试系统验证、自启启用或真实全局快捷键。

## 尚需目标 runner 执行

现有 CI 的三个目标已接入 `node scripts/verify-dev-installation.mjs --run`（Linux 经 Xvfb）。验收会在临时 runner 上真实安装、启动、协调退出、重复卸载并核对测试数据保留。Windows 继续验证真实 NSIS 解包。接入时未触发远程工作流，三平台完整安装/卸载结果以实际 run 为准；本机没有 Windows/Linux 原生验收，也没有启用并移除已注册 macOS 登录项的实验。

发布、签名公证和更新 Feed 未在本任务中修改或发布。macOS 本地签名不等于 Developer ID 公证，Windows 安装包仍以项目现有签名状态为准。

## macOS 卸载失败与 Dev 图标回归

同日在唯一 bundle ID 的临时包复现 `--quit-for-uninstall` 失败：`SMAppService.mainApp.status` 为 `NotFound`（3），注销返回 `SMAppServiceErrorDomain` / 1，提示 `Operation not permitted`。修复后 `NotRegistered` / `NotFound` 无需注销，`Enabled` / `RequiresApproval` 仍必须注销成功；未知状态及真实注销失败不会被忽略。同时补齐 MyGo 内置简体中文资源。

- `go test -race ./desktop ./i18n ./assets` 和相应 `go vet` 通过。
- `PASSPINE_TEST_CLIENT_APP="<绝对路径>/dist/dev/Passpine Dev.app" go test -race ./desktop -run TestDevelopmentUninstall -v`：真实 bundle 回归通过。测试复制程序、随机生成系统标识和隔离数据，验证重复执行及数据保留；没有设置变量时明确跳过原生用例。
- 独立原生测试包启动虚构预览数据及空闲 stdio Host 后，执行卸载准备两次：主程序和 Host 均正常退出（0），控制端点消失，数据与配置保留，中文资源警告消失。没有卸载或替换现有安装。
- `node --test scripts/client-installation.test.mjs scripts/build-channel.test.mjs`：14 项通过；Windows/Linux 交叉编译通过。
- 两种风格的 Dev 字号由图标边长的 14% 调整为 21%，加粗并增加底板留白，重新生成全部 PNG/ICNS 尺寸；生成脚本哈希纳入缓存校验。`make client:build` 和 macOS bundle 签名校验通过。

提交前 review 补充了两项回归：快速重新安装后，新辅助进程会忽略前一次卸载留下的退出标记；Linux 进程核对改用 `/proc/<pid>/exe`，避免 `ps comm` 仅返回进程名称导致漏判，并识别已删除但仍运行的程序路径。生成器固定使用 LF 换行，避免 Windows 检出改变脚本哈希而触发不必要的重新生成。最终再次运行完整检查（工具脚本 83 项，77 通过 / 6 因缺少打包工具跳过）、Go race / vet、Swift、浏览器类型检查与 291 项测试，以及客户端与浏览器构建，均未发现新的失败。
