# Passpine Neo

用 Go 重写的 pass 兼容密码管理器，以及 WXT 浏览器伴侣。客户端通过 [MyGo](https://github.com/egoist/mygo) 的原生 GPU UI 和 [Onevra](https://github.com/oneo-me/onevra) 组件构建；不包含 WebView 前端或 Rust 应用代码。浏览器使用 Vite、WXT、TypeScript、React 和 Radix Themes，采用相同的 Onevra 暖白 / 暖炭配色。

## 目录

```text
client/                  Go 模块
  cmd/passpine/          原生客户端入口
  cmd/passpine-host/     浏览器 Native Messaging 辅助进程
  desktop/               Onevra 组件组合、窗口和原生系统集成
  app/                   客户端 / 扩展共用的单一会话、配置与协议
  vault/                 OpenPGP、pass 文件库、恢复、Git、密码生成和 TOTP
  nativehost/            持久 stdio / 本机通信、身份验证和 Host 注册
  deviceunlock/          macOS 系统解锁桥接
  native/macos/          Secure Enclave 与系统验证辅助程序
browser/                 WXT + Vite + TypeScript 扩展
  src/components/        可复用账号列表、字段、表单、生成器和连接状态
  src/lib/               协议、生命周期、网站匹配和自动填充
  entrypoints/           popup、options 和 background
  preview/               独立、仅虚构数据的交互演示
scripts/                 构建和完整检查入口
docs/                    架构、功能边界和验证记录
```

## 直接体验

本机 macOS Apple Silicon 构建产物位于 `dist/Passpine Neo.app`。演示启动：

```sh
open -n 'dist/Passpine Neo.app' --args --preview
```

演示会新建隔离临时目录、真实 OpenPGP 密钥和 11 条虚构账号，不读取个人密码库，不注册浏览器 Host 或全局快捷键。演示口令为 `Passpine-demo-2026`。关闭演示后临时目录保留，可按应用输出 / 进程参数定位；真实应用和演示的配置分开。

浏览器交互预览：

```sh
npm --prefix browser ci
npm run preview:browser
```

访问 `http://127.0.0.1:5174/`，可以体验搜索、详情、编辑、生成器、锁定和断线状态。这个网页使用独立的示例客户端，**不是**浏览器 Native Messaging 的替代运行模式；正式扩展不会在连接失败时回退到示例数据。

## 构建与检查

需要 Make、Go 1.27.1+、Node.js 22.12+ 和 npm。macOS 构建还需要 Xcode Command Line Tools（Swift 编译器）。首次打开已有 pass 密码库需要系统 GPG，自动读取匹配密钥后，日常加解密由 Go 完成；Git 同步使用系统 `git` 和已有 SSH / HTTPS 凭据。

```sh
make                            # 原生客户端 + Chrome / Firefox 扩展
make client:build               # 只构建客户端和 Native Messaging Host
make browser:build              # 只构建 Chrome / Firefox 扩展
make client:install             # 构建、安装客户端并注册浏览器连接
make browser:install            # 构建并安装配置所选的浏览器扩展
make install                    # 构建并安装客户端和配置所选的扩展
make help                       # 查看命令及可配置参数
bash scripts/check.sh            # Go race/vet、真实Host进程、TS、扩展测试
npm run package:browser          # Chrome / Firefox ZIP
npm run preview                  # 从源码运行隔离原生演示
npm run dev                      # 从源码运行正式客户端
```

Make 自动安装或更新浏览器构建依赖。`client:` 和 `browser:` 前缀分别限定客户端和扩展，不加前缀的 `build` / `install` 同时处理两者；旧的 npm 构建命令仍可使用。

macOS 脚本生成独立的 `.app`，内含 Go 客户端、Go Host 和 Swift 系统解锁辅助程序，使用本地 ad-hoc 签名。客户端默认安装到 `~/Applications/Passpine Neo.app`，Linux 安装到 `~/.local/bin`，安装后自动注册 Native Messaging Host。可通过 `make client:install CLIENT_INSTALL_DIR="/Applications"` 指定安装目录。分发到其他 Mac 时，系统可能要求确认本地构建的应用；本项目未配置 Developer ID 公证。

## 正式使用与原库迁移

1. 打开客户端，选择创建新库、打开本地库，或先下载 Git 仓库。首次启动会识别包含 `.gpg-id` 的默认密码库 `~/.password-store`，自动进入导入页并填入目录。新库口令至少 12 个字符。
2. 打开现有库时选择密码库目录并输入系统 GPG 私钥口令。客户端根据根目录及子目录的 `.gpg-id` 自动匹配本机密钥，无需选择恢复私钥文件；首次打开前会实际验证所有现有条目和嵌套收件人策略。
3. `.gpg` 与 `.gpg-id` 保持 pass 格式。没有专用数据库转换；可与 GnuPG / pass 互通。私钥和恢复副本保存在应用数据目录，不放入同步仓库。
4. 导出恢复私钥后，使用“验证恢复备份”验证其可解密当前库。备份密码库目录、恢复私钥和口令。
5. 如需同步，在设置中初始化 Git、保存远端地址并手动同步。确认远端认证后可开启变更后同步及定时同步。保存期间遇到同步锁会提示重试。

系统 GPG 自动匹配支持指纹、密钥 ID、子钥指纹及邮箱；匹配不唯一时会报错。客户端通过 GPG 在内存中读取匹配的可导出私钥及其他收件人的公钥，验证后将私钥用输入口令加密保存到应用数据目录。口令不进入命令参数或日志，不生成明文私钥临时文件；遵循 `GNUPGHOME`，macOS 从 Finder 启动也会查找 Homebrew GPG。不同口令的多份私钥需要先统一口令；硬件卡私钥暂不支持导入。导入后的解锁使用客户端保存的加密副本，系统密钥轮换后需重新导入更新密钥。

macOS 默认配置目录：`~/Library/Application Support/Passpine Neo/`。可用 `--data-dir` 或 `PASSPINE_NEO_HOME` 指定隔离路径；自定义路径启动时，浏览器 Host 也需要相同的环境变量。正式使用推荐默认路径。

## 安装浏览器扩展

[本地安装、配置与浏览器加载说明](docs/install-local.md)

## 已实现的主要流程

- 多密码库、新建 / 导入 / 下载、口令解锁、自动锁定、锁屏 / 休眠锁定。
- 本地即时搜索、文件夹和密码条目列表、默认隐藏密码、原生剪贴板与定时清理。
- 锁定时默认列出名称；支持的 Mac 默认启用 Touch ID / 登录密码解锁，首次通过私钥口令解锁后完成系统验证。
- 设置使用整个窗口，时间参数采用分段选择；更改自动保存并短暂显示“已保存”，恢复中心与操作记录从设置打开。
- 已完成系统解锁注册的密码库仅显示 Touch ID / 登录密码入口。
- 新建、编辑、重命名 / 移动、原文及自定义字段、加密草稿、加密回收站。
- 密码生成、TOTP、恢复私钥导出和实际恢复验证。
- Git 初始化 / 远端 / 同步 / 自动同步，锁定时仍可运行；仓库状态始终显示，自动同步开关只控制后台同步。
- 原生菜单、托盘、文件选择器、全局 `Control+Option+Space`、`Command+F` 搜索及 `Command+Shift+L` 锁定。
- 扩展搜索、网站优先、详情、复制、填充、编辑、改名和删除；与客户端共用解锁会话。
- 本机操作记录，只含时间、动作、来源和结果，最多保存最近 200 条。

完整边界见 [功能与验证记录](docs/VALIDATION.md)，通信与安全设计见 [架构说明](docs/ARCHITECTURE.md)。当前已验收 macOS Apple Silicon；Windows 端的安全文件访问和 Host 注册尚未实现。登录启动、独立悬浮快搜窗口及英文界面尚未迁移，不能把此版本视为旧客户端全部平台能力的逐项替代。
