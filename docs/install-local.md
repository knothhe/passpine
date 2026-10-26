# 本地安装

## macOS Homebrew 正式版

Apple Silicon Mac（macOS 13 Ventura 及以上）安装 [Homebrew](https://brew.sh/) 后运行：

```sh
brew install --cask knothhe/tap/passpine
```

完整名称会自动添加 [knothhe/tap](https://github.com/knothhe/homebrew-tap)，也可先执行 `brew tap knothhe/tap`。cask 下载 GitHub Releases 中的正式应用包，校验 SHA-256，安装 `Passpine.app` 并提供 `passpine` 命令，无需 Go、Node.js 或本地构建。当前正式版只提供 Apple Silicon 安装包，Intel Mac 不适用。

打开 Passpine 后，在设置 → 浏览器中修复所需浏览器的连接，再按下文加载扩展。macOS 应用尚未进行 Developer ID 公证；若系统阻止首次打开，请在系统设置 → 隐私与安全性中核对来源后允许打开。

```sh
brew update
brew upgrade --cask --greedy knothhe/tap/passpine
brew uninstall --cask knothhe/tap/passpine
```

客户端支持自动更新；`--greedy` 允许 Homebrew 同时检查这类应用。普通卸载保留密码库、配置及密钥。Dev 客户端仍使用下方 Make 流程，安装与数据目录独立。

旧的 `knothhe/passpine` tap 继续接收相同的配方更新，已有安装可以沿用旧命令。新安装使用 `knothhe/tap`。

## 从源码构建与安装 Dev 版

本地命令构建和安装 **Dev 客户端及统一浏览器扩展**。正式客户端由 GitHub Actions 在发布 tag 上构建，从 GitHub Release 下载安装；客户端本地安装脚本拒绝正式通道。Dev 与正式客户端使用不同的应用、数据目录及 Native Host，可同时安装。Dev 不检查正式更新，也不自动发现 `~/.password-store`；开发时请使用测试密码库。扩展只有一组名称和 ID，默认连接正式客户端；本地解包加载时可在设置中切换 Dev 客户端，商店安装只连接正式客户端。

需要 Make、Go 1.27.1+、Node.js 22.12+ 和 pnpm 12.11.2。macOS 还需要 Xcode Command Line Tools，构建会将 Swift 系统解锁辅助程序一并打包。在仓库根目录运行命令；Make 会按锁文件安装浏览器构建依赖。客户端和扩展通过不同前缀选择，无前缀的命令同时处理两者。

| 操作 | 客户端 | 浏览器扩展 | 两者 |
| --- | --- | --- | --- |
| 构建 | `make client:build` | `make browser:build` | `make` 或 `make build` |
| 构建并安装 | `make client:install` | `make browser:install` | `make install` |
| 卸载 Dev 客户端（保留数据） | `make client:uninstall` | 不卸载扩展 | `make uninstall` |

## 卸载 Dev 客户端

`make client:uninstall`（或 `make uninstall`）默认只处理 Dev 客户端；自定义安装目录时继续传入安装时使用的 `CLIENT_INSTALL_DIR`。不会构建程序或卸载浏览器扩展，不清除密码库、配置、密钥及凭据，也不移除正式版。

macOS/Linux 的本地安装会在安装父目录写入 `.passpine-dev-install.json`，记录精确路径和程序/桌面资源的内容哈希。卸载先核对归属、通道、路径及内容，再请求该安装正常退出；未保存编辑沿用客户端的确认弹窗，写入失败或等待超时会停止本次卸载。通过独立控制端点匹配完整可执行路径，不会向另一安装发送退出请求；Linux 通过 `/proc/<pid>/exe` 识别仍在运行的程序。Dev 浏览器辅助进程的消息循环收到仅匹配自身路径、有效期一分钟且晚于本次启动的私有退出记录后，清理 relay 连接并正常结束进程，不依赖关闭阻塞中的标准输入。快速重新安装后启动的辅助进程忽略旧退出记录。

只删除仍指向该安装的 Native Host 清单和自有桌面资源；发生部分清理失败时保留安装记录，修正问题后重跑同一命令。记录缺失、内容被替换、符号链接或安装与数据目录重叠时保留文件并报错。旧版和手动复制的应用没有记录，请先用 `make client:install` 在相同位置重新安装一次，再使用卸载入口。

macOS 注销开机自启前会检查系统服务状态；未注册或找不到服务时无需注销，已注册或等待批准的服务仍须清理成功。早期 Dev 卸载实现会对不存在的服务报 `Operation not permitted`；由于退出命令来自已安装的程序，需先运行一次 `make client:install` 更新该程序，再重试 `make client:uninstall`，仅更新源码或构建目录不会替换旧安装。

Windows 沿用 NSIS 卸载器，Make 入口读取当前用户的 Dev 安装记录，检查通道、目录及 reparse point，再协调退出和卸载。注册项只有仍归属当前安装时才清理；失败保留可重试的安装记录。卸载会撤销对应客户端的开机自启注册。

现有 CI 已接入 `node scripts/verify-dev-installation.mjs --run`，仅在临时 runner 上执行真实 Dev 安装、原生启动、正常退出、重复卸载和数据保留检查；Linux 使用 Xvfb。脚本接入不代表尚未运行的目标系统已验收，结果以对应 workflow run 为准。

## 客户端

macOS Dev 产物为 `dist/dev/Passpine Dev.app`，安装到 `~/Applications/Passpine Dev.app`；Linux 将 Dev 程序以 `passpine-dev` 和 `passpine-dev-host` 名称安装到 `~/.local/bin`。客户端安装完成后，使用安装位置中的 Dev Host 注册独立浏览器连接。正式安装目标不参与本地替换。

Linux 安装同时在 `${XDG_DATA_HOME:-~/.local/share}` 下创建 `applications/com.knothhe.passpine.dev.desktop` 和应用图标，可从桌面应用菜单启动。`CLIENT_INSTALL_DIR` 可指定二进制安装目录，菜单入口会记录该目录的绝对路径。客户端使用 MyGo 原生 UI，运行需要 GTK 3，无需 WebKitGTK；Arch Linux 可运行 `sudo pacman -S --needed gtk3`。完整测试还需要 `zip` / `unzip`。托盘使用 StatusNotifierItem 协议，桌面面板需支持该协议；左键执行设置中的托盘动作，右键显示菜单。

Hyprland 0.55+ 会话通过 `hyprctl` 的 Lua 接口注册当前搜索快捷键，运行时绑定随客户端退出清理，配置重载后自动恢复，不修改用户配置文件；发生冲突时保留已有绑定并报告失败。需要 `hyprctl` 和 `busctl`（Arch 的 Hyprland / systemd 已提供）。其他 Linux 桌面沿用 X11 或 GlobalShortcuts Portal；Wayland 下的按键授权由桌面管理。

Linux 的快捷键录制在 GTK 原生窗口捕获按键，录制期间申请键盘捕获，避免菜单或桌面快捷键提前处理组合键；保存、取消、窗口失焦或关闭时立即释放。支持字母、数字、F1–F24 及常见特殊键，无法识别的按键显示校验提示。原生录制测试还需要 `wtype`：`PASSPINE_NATIVE_SMOKE=1 go test ./desktop -run TestNativeLinuxShortcutRecording -count=1 -v`。

Linux 默认使用 Cairo 软件渲染，避开当前 MyGo 0.5.5 在多窗口切换时因 OpenGL 上下文失效而退出的问题。可显式设置 `MYGO_GPU=1` 测试 GPU 路径，但当前版本尚未验证该路径的多窗口稳定性。

Windows 还需安装 NSIS 3（例如 `choco install nsis`）。Make 根据系统自动分流；Windows 使用系统 PowerShell，无需 Bash。`make build` 同时生成 `dist/dev/` 内的 Dev 程序、`Passpine-Dev-<版本>-windows-amd64-setup.exe` 和统一的 Chrome/Firefox 扩展。`make client:install` 静默运行安装包，默认安装到 `%LOCALAPPDATA%\Programs\Passpine Dev`；Windows 的 `CLIENT_INSTALL_DIR` 指应用目录本身，例如 `make client:install CLIENT_INSTALL_DIR="C:\Apps\Passpine Dev"`。`make install` 与直接运行 setup.exe 使用相同的默认目录；未指定目录时会沿用 Dev 注册表中记录的上次安装位置；Dev 和正式版的安装、卸载及浏览器注册表项互相独立。客户端安装成功后，Make 安装命令会从注册表读取并输出实际安装目录。

Windows 安装包使用当前用户权限，创建开始菜单快捷方式和系统卸载入口，为已安装的 Chrome、Edge、Brave 和 Firefox 写入 HKCU Native Messaging 注册表项。卸载会清理本安装的注册项和程序文件，保留密码库、密钥及用户数据。更新前应退出客户端。安装包没有 Authenticode 签名；Windows 支持使用当前账户的登录密码进行系统解锁；修改登录密码后，在安全设置中关闭系统解锁，用私钥口令打开密码库，再重新开启并锁定、使用私钥口令解锁以完成注册。

Windows 构建会从现有黑白图稿生成多尺寸图标并嵌入主程序、安装程序和卸载程序，无需额外安装资源编译器。图标与代码签名相互独立。开机自启通过当前用户的 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` 注册，普通偏好设置及浏览器连接注册也不要求 Authenticode 签名；应在固定安装位置运行后启用自启，移动程序后重新关闭再开启。未签名程序可能触发 SmartScreen 提示，开启 Smart App Control 或企业应用控制策略时也可能被阻止执行，进而影响实际自启；签名也不保证立即获得 SmartScreen 信誉。参见 [Microsoft 的签名与信誉说明](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation)。

Windows 回归验证可在 PowerShell 中运行：

```powershell
Push-Location client
go test -timeout 120s ./...
go vet ./...
# 在有桌面的 Windows 会话中，打开隔离的演示窗口、验证渲染后自动退出。
$env:PASSPINE_NATIVE_SMOKE = '1'
go test ./cmd/passpine -run TestNativePreviewStartup -count=1 -v
Remove-Item Env:PASSPINE_NATIVE_SMOKE
Pop-Location
pnpm run test:windows-resources
pnpm run test:tooling
pnpm run test:browser
make build
```

常规客户端测试包含临时密码库的新建、编辑、加密落盘、锁定/解锁和重新打开验证。原生启动测试使用临时数据及独立单实例目录，不连接已运行的正式客户端。Windows 私有文件权限按受保护的 DACL 验证；缺少文件符号链接权限时，仅跳过对应符号链接用例，目录跳转使用 junction 测试。工具测试在 Windows 跳过 macOS ZIP 发布与 Unix shebang 模拟程序用例；tag release 通过 GitHub Actions 分别在 macOS、Windows、Linux 上构建成品包，见[版本与发布](versioning.md)。

```sh
make client:install
make client:install CLIENT_INSTALL_DIR="/Applications"
```

macOS / Linux 的 `make client:install` 安装后会在 Dev 数据目录的 `test-vault/` 中创建虚构的测试密码库（含演示条目并初始化 Git），口令为 `Passpine-demo-2026`。已存在时跳过；已有其他密码库时不改变当前选择，可在客户端切换或通过 `PASSWORD_STORE_DIR` 供命令行使用。Dev 客户端运行时不会创建，需退出后运行 `passpine-dev --seed-test-vault`（macOS 为应用包内的 `Contents/MacOS/passpine`）；设置 `PASSPINE_SKIP_TEST_VAULT=1` 可跳过。

`CLIENT_INSTALL_DIR` 是安装父目录，需对其有写权限。macOS 更新会替换应用包，不保留旧版应用资源。关闭正在运行的 Dev 客户端后再更新并重新启动。当前已验收 macOS Apple Silicon；Linux 已在 Arch Linux 的 Wayland 桌面验证构建、安装、Host 注册和隔离演示窗口渲染，其他发行版仍需验证。

Linux 桌面会话中可运行原生启动测试（使用临时演示库，渲染后自动退出）：

```sh
cd client
PASSPINE_NATIVE_SMOKE=1 go test ./cmd/passpine -run TestNativePreviewStartup -count=1 -v
# Hyprland：验证快捷键注册/冲突/注销及快捷面板交互
PASSPINE_NATIVE_SMOKE=1 go test ./internal/linuxdesktop ./desktop -run 'TestHyprlandRuntimeShortcut|TestNativeLinuxInteractions' -count=1 -v
```

Dev 应用 ID 为 `com.knothhe.passpine.dev`，名称为 `Passpine Dev`。数据目录为系统配置目录下的 `Passpine Dev`，例如 macOS 的 `~/Library/Application Support/Passpine Dev/`；仅 `PASSPINE_DEV_HOME` 可覆盖 Dev 默认数据目录，正式版继续使用 `PASSPINE_HOME`。默认全局快捷键为 `Ctrl+Alt+Shift+Space`，与正式版的 `Ctrl+Alt+Space` 分开。密码库和应用数据保存在应用包之外。

手动移动应用后，在客户端设置 → 浏览器中逐项**修复**，或选择**重新检测并修复**。只为已安装的浏览器注册连接；不要注册临时构建目录里的应用后删除该目录。

## 扩展配置

扩展安装通过 `make browser:install` 复用 `h2-browser-extension` skill 的本地安装流程：构建所选浏览器的 MV3 版本，复制到固定的 `<安装父目录>/<插件名称>`。配置文件放在仓库根目录。

```sh
cp .install-local.example.json .install-local.json
```

编辑 `.install-local.json`：

```json
{
  "browser": "chrome",
  "name": "passpine",
  "directories": {
    "chrome": "~/ChromeExtensions",
    "firefox": "~/FirefoxExtensions",
    "edge": "~/EdgeExtensions"
  }
}
```

个人配置已被 Git 忽略。示例中的 Chrome 安装目标为 `~/ChromeExtensions/passpine`，Firefox 为 `~/FirefoxExtensions/passpine`。安装使用配置原名，不再自动添加 `-dev`；已有显式 `-dev` 名称仍按原名保留，个人配置文件不会被重写。父目录支持 `~/`，相对路径以仓库根目录为基准；不展开 `$HOME` 等环境变量。省略名称时使用包名 `passpine`。名称只决定本地子目录，不改变扩展 ID 或 manifest 名称。不会自动迁移、清理旧 Dev 插件目录或浏览器中的旧扩展。

```sh
make browser:install
make browser:install BROWSER=firefox
make browser:install BROWSER=edge
make install

# 无配置文件时，直接提供安装父目录
make browser:install BROWSER=chrome INSTALL_ARGS='--dir "~/ChromeExtensions" --name passpine'

# CLI 选项优先于配置；也可选择另一份配置
make browser:install INSTALL_ARGS='--config .install-local.custom.json'
make browser:install BROWSER=firefox INSTALL_ARGS='--dir "~/FirefoxExtensions" --name passpine'
node scripts/install-local.mjs --help
```

自定义配置含个人路径时，也应加入 Git 忽略。没有配置、也没有 `--dir` 时，安装命令会报错并提示配置；仅构建不需要安装配置。

## 构建映射与更新

`make browser:build` 构建 Chrome 和 Firefox。单独构建使用 `make browser:build:chrome`、`make browser:build:firefox` 或 `make browser:build:edge`。Chrome / Edge 共用 `browser/.output/chrome-mv3`，Firefox 使用 `browser/.output/firefox-mv3`；本地与 CI 使用同一扩展身份和输出路径。安装只构建所选浏览器，默认使用配置中的 `browser`，省略时为 Chrome。

沿用 skill 的可选配置：`package_manager` 兼容 `npm` / `pnpm`，本项目依据根目录 `packageManager` 默认使用 `pnpm`；`build_scripts` 按浏览器指定仓库根目录 `package.json` 的脚本名；`output_directories` 指定对应的 MV3 解包目录。默认构建脚本为 `build:browser:chrome`、`build:browser:firefox`、`build:browser:edge`，均调用 Make。

安装只替换具名插件子目录，保留父目录文件和其他插件。新目录、空目录以及同一项目、同一浏览器的 schema 2 受管目录可直接更新；其他非空目录先显示完整目标路径，并要求确认覆盖 `[Y/n]`。输入回车 / `Y` / `yes` 确认，`n` / `no` 或关闭输入取消。插件目录中的旧资源会被删除，请不要混放个人文件。

目标不能与仓库重叠，不能是符号链接，名称必须是单个合法目录名。脚本在构建前检查目标，构建和 manifest 校验成功后再通过同级临时目录替换；构建或复制失败保留原安装，替换失败尝试恢复，恢复失败时输出并保留备份路径。

## 首次加载与使用

1. 安装并打开正式客户端，或运行 `make client:install` 安装 Dev 客户端，再创建或打开密码库。
2. Chrome 在 `chrome://extensions`、Edge 在 `edge://extensions` 开启开发者模式，加载配置中的固定插件子目录。Brave 可加载同一 Chromium 产物。Firefox 在 `about:debugging#/runtime/this-firefox` 临时加载固定目录中的 `manifest.json`，重启浏览器后需再次加载；商店分发需要 Firefox 签名。
3. 更新后，在扩展管理页重新加载扩展。同步到安装目录不代表浏览器已加载新版本。
   本地解包加载时，扩展设置 → 桌面客户端可选择正式版或 Dev 版，默认正式版。商店安装固定连接正式版，不显示 Dev 选择。目标切换会清除旧会话及未保存输入，不会自动连接另一个客户端。
4. 支持的 Mac 和 Windows 默认启用系统解锁，首次用私钥口令打开密码库后完成系统验证：Mac 使用 Touch ID 或登录密码，Windows 使用当前账户的登录密码。扩展锁定时操作条目会自动请求系统验证，成功后继续操作；尚未启用系统解锁时会唤出客户端。客户端关闭则连接断开，可在客户端开启“关闭窗口后继续运行”。
5. 点击账号正文直接填充，右侧详情图标用于查看。无需提前关联网站，成功填充后默认关联当前精确 hostname 并自动关闭弹窗；关联保存失败但网页已填充时同样关闭。再次打开后可在更多菜单或详情中取消关联，再次成功填充会重新关联。关联不自动继承子域，填充不会提交表单；无可填写表单时保留弹窗显示错误，且不会新增关联。独立 demo 保留弹窗和关联保存失败提示，用于检查界面状态。

Chrome / Edge 扩展 ID 统一为 `aphkddfagflppihmghlcjpnkkdgbflnp`，Firefox ID 为 `passpine@knothhe.com`。正式 Host 为 `com.knothhe.passpine`，Dev Host 为 `com.knothhe.passpine.dev`，各自继续使用独立数据目录。更新后的 Dev 客户端授权同一扩展 ID；旧 Dev 客户端需先更新，再在客户端设置中修复浏览器连接。产品版本号不要求一致，只要通信协议兼容即可。

## Agent 收尾安装

使用 `h2-browser-extension` skill 完成执行型扩展任务后，若仓库根目录存在真实 `.install-local.json`，会按配置执行 `make browser:install` 并报告结果；示例文件、纯讨论或用户明确要求跳过时不触发。覆盖确认仍由用户选择。

本项目尚未配置 Codex 项目级收尾 hook。可选 hook 会在存在真实配置时，于 Codex 回复结束前触发一次安装检查，再由 Codex 完成安装并提醒重新加载；仅对当前项目生效，需要明确同意后接入。
