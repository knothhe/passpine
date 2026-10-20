# 本地安装

需要 Make、Go 1.27.1+、Node.js 22.12+ 和 pnpm 12.11.2。macOS 还需要 Xcode Command Line Tools，构建会将 Swift 系统解锁辅助程序一并打包。在仓库根目录运行命令；Make 会按锁文件安装浏览器构建依赖。客户端和扩展通过不同前缀选择，无前缀的命令同时处理两者。

| 操作 | 客户端 | 浏览器扩展 | 两者 |
| --- | --- | --- | --- |
| 构建 | `make client:build` | `make browser:build` | `make` 或 `make build` |
| 构建并安装 | `make client:install` | `make browser:install` | `make install` |

## 客户端

macOS 构建产物为 `dist/Passpine.app`，安装到 `~/Applications/Passpine.app`；Linux 将 `dist/passpine` 和 `dist/passpine-host` 安装到 `~/.local/bin`。客户端安装完成后，使用安装位置中的 Host 注册浏览器连接。

```sh
make client:install
make client:install CLIENT_INSTALL_DIR="/Applications"
```

`CLIENT_INSTALL_DIR` 是安装父目录，需对其有写权限。macOS 更新会替换应用包，不保留旧版应用资源。关闭正在运行的客户端后再更新并重新启动。当前已验收 macOS Apple Silicon，Linux 客户端运行能力仍需在目标系统确认。

macOS 应用 ID 为 `com.knothhe.passpine`，与旧原生客户端相同；安装到同一目录会替换原有 `Passpine.app`。本机数据不会随应用包安装而迁移，已有 Go 配置的继续使用与迁移方式见 [README](../README.md#正式使用与原库迁移)。

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

个人配置已被 Git 忽略。示例中的 Chrome 安装目标为 `~/ChromeExtensions/passpine`，Firefox 为 `~/FirefoxExtensions/passpine`。父目录支持 `~/`，相对路径以仓库根目录为基准；不展开 `$HOME` 等环境变量。省略名称时使用根目录 `package.json` 中的 `name`（本项目为 `passpine`），未声明包名时才使用仓库目录名。名称只决定本地子目录，不改变扩展 ID 或 manifest 名称。

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

`make browser:build` 构建 Chrome 和 Firefox。单独构建使用 `make browser:build:chrome`、`make browser:build:firefox` 或 `make browser:build:edge`。Chrome / Edge 共用 `browser/.output/chrome-mv3`，Firefox 使用 `browser/.output/firefox-mv3`。安装只构建所选浏览器，默认使用配置中的 `browser`，省略时为 Chrome。

沿用 skill 的可选配置：`package_manager` 兼容 `npm` / `pnpm`，本项目依据根目录 `packageManager` 默认使用 `pnpm`；`build_scripts` 按浏览器指定仓库根目录 `package.json` 的脚本名；`output_directories` 指定对应的 MV3 解包目录。默认构建脚本为 `build:browser:chrome`、`build:browser:firefox`、`build:browser:edge`，均调用 Make。

安装只替换具名插件子目录，保留父目录文件和其他插件。新目录、空目录以及同一项目、同一浏览器的 schema 2 受管目录可直接更新；其他非空目录先显示完整目标路径，并要求确认覆盖 `[Y/n]`。输入回车 / `Y` / `yes` 确认，`n` / `no` 或关闭输入取消。插件目录中的旧资源会被删除，请不要混放个人文件。

目标不能与仓库重叠，不能是符号链接，名称必须是单个合法目录名。脚本在构建前检查目标，构建和 manifest 校验成功后再通过同级临时目录替换；构建或复制失败保留原安装，替换失败尝试恢复，恢复失败时输出并保留备份路径。

## 首次加载与使用

1. 运行 `make client:install`，打开安装后的客户端并创建或打开密码库。
2. Chrome 在 `chrome://extensions`、Edge 在 `edge://extensions` 开启开发者模式，加载配置中的固定插件子目录。Brave 可加载同一 Chromium 产物。Firefox 在 `about:debugging#/runtime/this-firefox` 临时加载固定目录中的 `manifest.json`，重启浏览器后需再次加载；商店分发需要 Firefox 签名。
3. 更新后，在扩展管理页重新加载扩展。同步到安装目录不代表浏览器已加载新版本。
4. 支持的 Mac 默认启用系统解锁，首次用私钥口令打开密码库后完成 Touch ID 或登录密码验证。扩展锁定时操作条目会自动请求系统验证，成功后继续操作；尚未启用系统解锁时会唤出客户端。客户端关闭则连接断开，可在客户端开启“关闭窗口后继续运行”。
5. 点击账号正文直接填充，右侧详情图标用于查看。无需提前关联网站，成功填充后默认关联当前精确 hostname 并自动关闭弹窗；关联保存失败但网页已填充时同样关闭。再次打开后可在更多菜单或详情中取消关联，再次成功填充会重新关联。关联不自动继承子域，填充不会提交表单；无可填写表单时保留弹窗显示错误，且不会新增关联。独立 demo 保留弹窗和关联保存失败提示，用于检查界面状态。

Chrome / Edge 固定扩展 ID 为 `obnlebeedcbbchadakacilopfdaifgjh`，Firefox ID 为 `passpine@knothhe.com`，Native Host 为 `com.knothhe.passpine`。Chromium ID 保持不变；Firefox ID 与旧插件相同，新版会替换对应扩展。更换客户端后，重新加载扩展并在客户端设置中修复浏览器连接。

## Agent 收尾安装

使用 `h2-browser-extension` skill 完成执行型扩展任务后，若仓库根目录存在真实 `.install-local.json`，会按配置执行 `make browser:install` 并报告结果；示例文件、纯讨论或用户明确要求跳过时不触发。覆盖确认仍由用户选择。

本项目尚未配置 Codex 项目级收尾 hook。可选 hook 会在存在真实配置时，于 Codex 回复结束前触发一次安装检查，再由 Codex 完成安装并提醒重新加载；仅对当前项目生效，需要明确同意后接入。
