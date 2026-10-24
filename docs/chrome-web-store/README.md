# Chrome Web Store 上架材料

目标：Passpine **0.1.2**，Chrome Manifest V3。准备日期：**2026-10-22**。本目录是提交材料，不包含商店上传或审核提交记录。

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

- 已核对构建后的 `browser/.output/chrome-mv3/manifest.json`：MV3、0.1.2、五项权限、无常驻站点注入和广泛主机权限。权限及数据流说明按实际代码编写。
- 中英文短描述沿用包内 `_locales`；名称和描述均低于官方 75 / 132 字符限制。两种介绍的功能、前提和数据处理含义保持一致。
- Chrome 官方图片、介绍、隐私和发布文档已在 2026-10-22 联网核对，见 [来源记录](sources.md)。尚未访问发布者后台。
- 浏览器类型检查、231 项测试、Chrome / Firefox 构建通过。截图在当前生产扩展与隔离 Go 客户端之间实际完成 Native Messaging 连接、账号解密及密码生成；最终商店安装 ID + 网页填充、复制、锁定等完整验收仍需按审核指南复跑。

## 提交前的必要补项

1. **公开隐私政策。** 政策已作为官网 `/privacy` 页面实现（见 [PRIVACY.md](PRIVACY.md)）。部署官网并绑定正式域名后，验证该 HTTPS 地址无需登录即可访问，再填入商店后台。政策的联系章节提供 GitHub Issues（公开）和 knothhe@gmail.com（私密）两个渠道。现有公开支持入口为 [GitHub Issues](https://github.com/knothhe/passpine/issues)，不要在此提交秘密。
2. **配套客户端下载。** 在审核说明填入本版本公开、可直接获取的安装包固定 URL，说明平台与安装限制。源码仓库链接不等于已验证可用的安装包。
3. **商店扩展 ID。** 首次创建草稿后比较后台 Item ID 与当前开发 ID `obnlebeedcbbchadakacilopfdaifgjh`。若不同，按 [官方流程](https://developer.chrome.com/docs/extensions/reference/manifest/key)同步扩展公钥及客户端 Host 允许来源，重新构建、安装并测试真实连接。不能假定开发 key 已绑定商店条目。
4. **最终实机验收。** 图片已生成并检查，均使用虚构数据。使用最终商店 ID 和对应客户端完成审核指南中的连接、复制、填充、锁定和恢复流程，并记录实际结果。
5. **后台信息。** 完成发布者账户要求、类别、分发范围、价格、数据类型、Limited Use 认证和测试说明。对 HTTP 填充等限制如实说明；根据实际网站和运营情况确认安全处理要求。

## 上传包与填写顺序

本次已生成 [Chrome 0.1.2 扩展 ZIP](package/passpine-0.1.2-chrome.zip)，可用于创建商店草稿。需要重新生成发行 ZIP 时，在仓库根目录运行：

```sh
pnpm run package:browser
```

该命令本地构建并打包 Chrome 与 Firefox，不会上传商店。Chrome 预期文件为 `browser/.output/passpine-0.1.2-chrome.zip`；上传前检查 ZIP 内 `manifest.json` 位于根目录、版本为 0.1.2、没有未替换文案占位符、没有源码/测试配置/真实凭证。不要把整个仓库或本目录的材料 ZIP 当扩展安装包。

随后可在开发者后台上传 Chrome ZIP 为草稿，先核对 ID，再填写中英文介绍、图片、隐私政策、权限与数据声明、分发和 Test instructions。全部检查完成后由发布者决定提交审核与发布方式。[官方首次发布流程](https://developer.chrome.com/docs/webstore/publish)。
