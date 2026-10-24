# Chrome Web Store 上架材料

当前商店审核版本：Passpine **0.1.9**，Chrome Manifest V3，发布者已提交，后台状态为 **Pending review**。准备日期：**2026-10-22**，身份与后台状态更新于 **2026-10-25**。现有截图来自 0.1.7；此后账号列表、详情和生成器界面未改动。

## 可填写的材料

| 文件 | 用途 |
| --- | --- |
| [listing.zh-CN.md](listing.zh-CN.md) | 简体中文名称、短描述、详细介绍和链接 |
| [listing.en.md](listing.en.md) | English 商店介绍 |
| [privacy-permissions.md](privacy-permissions.md) | 单一用途、逐项权限理由、远程代码、数据类型与代码证据 |
| [PRIVACY.md](PRIVACY.md) | 指向官网 `/privacy` 公开隐私政策的说明 |
| [reviewer-instructions.md](reviewer-instructions.md) | 中英文审核步骤、配套客户端前提及虚构测试表单 |
| [sources.md](sources.md) | 已访问的 Chrome 官方来源、当前要求及本地事实来源 |

图片、拍摄环境和复现命令见 [素材记录](assets.md)。已提供中英文各三张实际扩展截图、128px 图标、440×280 小宣传图及可选 1400×560 Marquee。可直接检查 [中文截图](assets/zh-CN/01-accounts.png) 与 [English 截图](assets/en/01-accounts.png)。

## 当前核对状态

### 2026-10-25：商店身份与连接

- 商店 Item ID 为 `aphkddfagflppihmghlcjpnkkdgbflnp`。从后台 Package → View public key 取得公钥，按 SHA-256 推导 ID，已同步 `config/browser-identities.json`、客户端 `ReleaseChromiumID`、本地预览与截图脚本。原生注册与来源校验只接受新 ID；旧开发 ID `obnlebeedcbbchadakacilopfdaifgjh` 的拒绝及注册修复已有回归覆盖。
- 使用后台下载的 0.1.9 草稿 CRX 验证，SHA-256 为 `c0f0c04b0b0b48d26796652cfcaf43ef08511a371370b7df6ee254cfe2ec8b42`。其业务文件与当前商店 ZIP 一致；商店额外添加 `update_url` 和 `_metadata`。在临时解包副本中仅补入商店公钥，以固定未发布扩展的测试 ID；没有安装到个人浏览器。
- macOS arm64、Chrome for Testing 153.0.8010.12、全新临时 profile、真实 Go `app.Service` / Native Host、隔离加密库和五个虚构账号：设置页检查连接、真实工具栏弹窗列出账号、解密详情、锁定后拒绝读取以及客户端退出后的断连提示均通过。Go fixture 使用正式 Host 通道，不是已发布客户端安装包；测试配置、库与密钥已清理。
- **公开客户端仍需更新。** 已发布的 v0.1.9 及此前客户端仍授权旧 ID。源码修正不会改变公开安装包，须按现有发布流程发布带本次修复的客户端、更新审核下载地址，并补做最终安装路径验收。发布者已将商店 0.1.9 提交审核；客户端下载地址将在修复版本发布成功后更新。Windows/Linux 原生连接未在本机验证。
- 中英文介绍、各三张截图、图标、宣传图、权限与数据声明以及测试说明已保存到后台草稿。分发沿用免费、公开和全部地区。官网为 <https://passpine.knothhe.com/>，隐私政策为 <https://passpine.knothhe.com/privacy>；后者已用不带凭据的请求确认 HTTP 200 并包含政策正文，两项已填入后台。

### 历史材料与构建

2026-10-24 工作区新增页内录制的网页快捷键，需要用户主动授权可选网站访问。商店截图记录对应此前构建；当前权限说明与官网政策源码已同步更新。同日修复商店上传报错 `key field is not allowed in manifest`：打包入口改用独立商店配置，Chrome ZIP 省略 `key`，本地解包构建保留固定公钥。历史 0.1.7 ZIP 含 `key`，不再用于上传。

- 历史 0.1.7 Chrome ZIP 的记录保留在本目录；其 `key` 虽与本地身份一致，仍被商店上传校验拒绝。新包须通过下方入口生成，并检查最终 ZIP 省略 `key`。商店分配的 ID 另行核对，不能从无 `key` 的 ZIP 推断。
- 中英文短描述沿用包内 `_locales`；名称和描述均低于官方 75 / 132 字符限制。两种介绍的功能、前提和数据处理含义保持一致。
- Chrome 官方图片、介绍、隐私和发布文档已在 2026-10-22 联网核对，2026-10-25 通过 Ego lite 复核并填写后台，见 [来源记录](sources.md)。
- 0.1.7：浏览器类型检查、291 项测试、Chrome / Firefox 构建与打包通过。中英文六张截图已于 2026-10-24 用 0.1.7 在 Linux 上重拍，在生产扩展与隔离 Go 客户端之间实际完成 Native Messaging 连接、账号解密及密码生成；字体与 macOS 版本的差异见 [素材记录](assets.md)。最终商店安装 ID + 网页填充、复制、锁定等完整验收仍需按审核指南复跑。

## 提交前的必要补项

1. **公开隐私政策（已完成）。** <https://passpine.knothhe.com/privacy> 已验证无需登录可访问并填入后台，见 [PRIVACY.md](PRIVACY.md)。政策提供 GitHub Issues（公开）和 knothhe@gmail.com（私密）两个渠道，不要在 Issues 提交秘密。
2. **截图与客户端下载。** 中英文截图已按 0.1.7 重拍（含新的“填充账号与密码 / Fill login”按钮），配套客户端下载地址已按 v0.1.7 Release 填入[审核说明](reviewer-instructions.md)。扩展尚未在商店发布，提交时若扩展版本已更新，同步重拍并更新这两项。
3. **商店扩展 ID（源码与隔离连接已修正）。** 商店、解包构建与客户端现统一为 `aphkddfagflppihmghlcjpnkkdgbflnp`，上传 ZIP 继续省略 `key`。发布修正后的客户端后，已有安装须在设置 → 浏览器修复 Host 注册；旧版客户端的来源校验也需要更新，单独编辑注册 JSON 无法完成迁移。
4. **最终实机验收。** 图片已生成并检查，均使用虚构数据。使用最终商店 ID 和对应客户端完成审核指南中的连接、复制、填充、锁定和恢复流程，并记录实际结果。
5. **后台信息。** 完成发布者账户要求、类别、分发范围、价格、数据类型、Limited Use 认证和测试说明。对 HTTP 填充等限制如实说明；根据实际网站和运营情况确认安全处理要求。

## 上传包与填写顺序

[Chrome 0.1.7 扩展 ZIP](package/passpine-0.1.7-chrome.zip)（历史 SHA-256 见 [SHA256SUMS](package/SHA256SUMS)）仅保留为旧构建记录，不能用于商店上传。旧 Release 附件不会随本次修复改变。生成当前源码的上传包时，在仓库根目录运行：

```sh
pnpm run package:browser
```

该命令本地构建并打包 Chrome 与 Firefox，不会上传商店。Chrome 预期文件为 `browser/.output/passpine-<版本>-chrome.zip`，构建目录为 `browser/.output/chrome-mv3-store`；上传前检查 ZIP 内 `manifest.json` 位于根目录、完全没有 `key` 字段、版本与目标一致、没有未替换文案占位符、没有源码/测试配置/真实凭证。不要直接运行不带商店配置的 `wxt zip`，也不要压缩本地安装目录；这两种方式会带入本地公钥。不要把整个仓库或本目录的材料 ZIP 当扩展安装包。

随后可在开发者后台上传 Chrome ZIP 为草稿，先核对 ID，再填写中英文介绍、图片、隐私政策、权限与数据声明、分发和 Test instructions。全部检查完成后由发布者决定提交审核与发布方式。[官方首次发布流程](https://developer.chrome.com/docs/webstore/publish)。
