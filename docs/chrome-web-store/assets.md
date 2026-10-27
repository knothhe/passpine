# 图片与拍摄记录

截图对应 Passpine 0.1.7，2026-10-24 在 Linux 上重新拍摄并逐张查看（0.1.2 版本于 2026-10-22 在 macOS 拍摄）。规格依据见[官方来源](sources.md)。所有账号和生成的示例密码仅用于演示；没有读取个人浏览器配置或密码库。

## 可上传的文件

| 文件 | 尺寸 / 格式 | 用途 |
| --- | --- | --- |
| [icon-128.png](assets/icon-128.png) | 128×128 PNG，保留透明度 | 必需商店图标；原样复用扩展当前黑白图标 |
| [promo-440x280.png](assets/promo-440x280.png) | 440×280 PNG，RGB 不透明 | 必需小宣传图；复用项目标识与英文产品定位 |
| [marquee-1400x560.png](assets/marquee-1400x560.png) | 1400×560 PNG，RGB 不透明 | 可选 Marquee 宣传图 |
| `assets/en/01-accounts.png`、`02-details.png`、`03-generator.png` | 每张 1280×800 PNG，RGB 不透明 | English 截图，按 01–03 顺序上传 |
| `assets/zh-CN/01-accounts.png`、`02-details.png`、`03-generator.png` | 每张 1280×800 PNG，RGB 不透明 | 简体中文截图，按 01–03 顺序上传 |

每种语言三张截图：账号列表、默认隐藏密码的详情、密码生成器。可在商店相应语言条目使用；不要把六张都放进同一个最多五张的截图槽位。宣传图两种语言共用。

[中文账号列表](assets/zh-CN/01-accounts.png) · [中文详情](assets/zh-CN/02-details.png) · [中文生成器](assets/zh-CN/03-generator.png)

[English accounts](assets/en/01-accounts.png) · [English details](assets/en/02-details.png) · [English generator](assets/en/03-generator.png)

## 实拍来源与验证范围

- 使用 0.1.7 的 `browser/.output/chrome-mv3` 生产构建，Linux 上 Playwright 提供的 Chromium 156.0.8078.4（品牌版 Chrome 不支持 `--load-extension`），独立临时 profile。通过真正的 `chrome.action.openPopup()` 工具栏弹窗入口拍摄，没有将 popup 当普通标签页打开或绕过消息来源校验。
- 隔离 Go 客户端服务由 [store-demo/main.go](../../scripts/store-demo/main.go) 创建：使用正式 `app.Service`、真实 OpenPGP 密钥/条目加密及 Native Messaging 转发；五个虚构账号保存在临时目录。关闭设备解锁、Git 同步和操作记录，不操作个人账户。测试结束关闭服务和浏览器，并删除测试 profile、密码库和私钥。
- 语言与浅色主题通过扩展真实设置控件切换。账号名称、目录和备注是用户数据，中文截图中保留其英文内容。
- 原始弹窗为 380×460，保存在 `source-screenshots/<语言>/`。最终图将完整实拍等比放大到 456×552，加入品牌底图和外围说明；没有重绘或修改扩展内部 UI。等待转场结束，检查无叠影、裁字或遮挡。
- 已实际验证真实连接、账号列表、解密详情、密码默认遮罩、本机密码生成及语言/主题设置。截图不展示网页填充，也不证明商店安装 ID、系统解锁、剪贴板到期或完整桌面窗口流程均已验收；提交前按[审核指南](reviewer-instructions.md)复跑。
- [capture-metadata.json](capture-metadata.json) 记录版本、浏览器、时间和场景。它不是提交审核记录。

## 重复生成

品牌图由 [generate-store-art.mjs](../../scripts/generate-store-art.mjs) 从 `client/assets/app-icon-monochrome.svg` 生成，商店图标直接复制 `browser/public/icon128-monochrome.png`。截图及合成由 [capture-store-screenshots.mjs](../../scripts/capture-store-screenshots.mjs) 生成，需要 Go、Sharp、Playwright 和支持加载解包扩展的完整 Chrome for Testing / Chromium。当前脚本的 Native Host 临时注册方式适用于 macOS / Linux，未验证 Windows。

```sh
pnpm --dir browser run build
# 已在本地安装 sharp / playwright 时可省略 PASSPINE_ART_NODE_MODULES。
PASSPINE_ART_NODE_MODULES=/path/to/node_modules node scripts/generate-store-art.mjs
PASSPINE_ART_NODE_MODULES=/path/to/node_modules \
PASSPINE_CAPTURE_BROWSER='/path/to/Chrome for Testing executable' \
node scripts/capture-store-screenshots.mjs
```

字体使用 Avenir Next / PingFang SC，回退至 Helvetica Neue / Arial；不同机器的外围字形可能不同。脚本只使用临时测试目录，不能传入个人浏览器 profile 或个人密码库。重新拍摄后逐张查看，并核对尺寸、语言与当前版本。

## 0.1.7 复核

对比 `v0.1.2..v0.1.7` 的扩展界面代码：

| 截图 | 0.1.7 是否需要重拍 | 原因 |
| --- | --- | --- |
| `01-accounts.png` | 否 | 账号行只修改了可访问名称和悬停提示，外观不变 |
| `02-details.png` | **是**（中英文） | 详情页右上角的填充图标改为带文字的“填充账号与密码 / Fill login”按钮；弹窗打开时没有可填充的网页，因此按钮按实际状态显示为禁用 |
| `03-generator.png` | 否 | 生成器组件未改动 |

商店图标与宣传图不受影响。脚本一次重拍全部六张，2026-10-24 已用 0.1.7 重拍，保证同一组截图字体与环境一致。

这次在 Linux 上拍摄，本机没有 Avenir Next / PingFang SC，底图文字与弹窗系统字体均回退为 Liberation Sans 及系统中文字体，字形不如 0.1.2 的 macOS 版本精致；搜索快捷键显示为 `Ctrl K`（macOS 为 `⌘ K`）。界面内容与 0.1.7 一致。如需与先前一致的字体，可在 macOS 上用同一命令重拍全部六张。

## 扩展 ZIP

`passpine-0.1.7-chrome.zip` 是历史本地构建记录，ZIP 本身不纳入版本控制，校验和见 [SHA256SUMS](package/SHA256SUMS)；`manifest.json` 位于 ZIP 根目录，无源码、测试服务、浏览器 profile 或私钥。该旧包包含 `key`，会被 Chrome Web Store 拒绝，不用于上传。

重新打包在仓库根目录执行 `pnpm run package:browser`，使用生成的 `browser/.output/passpine-<版本>-chrome.zip`；商店配置会省略 `key`，本地解包构建仍保留公钥。更新存档时只在 `package/SHA256SUMS` 记录新版本 ZIP 的校验和（ZIP 已被忽略，不提交），同步材料版本并重新核对。首次草稿创建后，仍需确认商店分配的 ID 与客户端允许来源一致。
