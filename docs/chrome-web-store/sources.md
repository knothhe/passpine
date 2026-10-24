# 官方要求与核对记录

核对日期：2026-10-22；“商店介绍字段”与“隐私字段”两页于 2026-10-23 复核，介绍页的 URL 字段（Official URL、Homepage、Support）均为可选，两页都没有服务条款字段。2026-10-24 复核 manifest key 与打包准备文档，并按实际上传报错修复 ZIP 中的 `key`。以下页面已通过联网工具访问；这是材料准备记录，不代表开发者后台已填写或审核已通过。

| 官方来源 | 本次核对结果 |
| --- | --- |
| [图片规范](https://developer.chrome.com/docs/webstore/images) | 必需：128×128 PNG 图标、440×280 小宣传图、至少 1 张截图（最多 5 张）。截图为 1280×800 或 640×400，直角、全幅；推荐前者。1400×560 Marquee 可选。图标可有透明留白；方形图案建议 96×96 居中。宣传图不按语言分别提供。 |
| [商店介绍字段](https://developer.chrome.com/docs/webstore/cws-dashboard-listing) | 填写详细描述、类别、语言及相关链接；可按包内 `_locales` 提供不同语言的描述与截图。 |
| [隐私字段](https://developer.chrome.com/docs/webstore/cws-dashboard-privacy) | 填写单一用途、逐项权限理由、远程代码及实际数据用途；声明必须与政策及产品行为一致。 |
| [用户数据 FAQ](https://developer.chrome.com/docs/webstore/program-policies/user-data-faq) | 仅本机处理也要披露。凭证、网站地址及表单数据属于需评估的数据。扩展与同机原生程序的传输有专门例外，不能据此声称其他传输均加密。 |
| [开发者政策](https://developer.chrome.com/docs/webstore/program-policies/policies) | 使用最小必要权限，清楚披露用途及接收方，提供准确可访问的隐私政策，并遵守 Limited Use。上架内容应反映实际功能。 |
| [首次发布](https://developer.chrome.com/docs/webstore/publish) | ZIP 上传、填写介绍/隐私/分发信息、提供 Test instructions、提交审核是不同步骤。可选择审核通过后手动发布。 |
| [manifest name](https://developer.chrome.com/docs/extensions/reference/manifest/name) / [description](https://developer.chrome.com/docs/extensions/reference/manifest/description) | name 最多 75 个字符，description 最多 132 个字符；本包两种语言均低于限制。 |
| [manifest key](https://developer.chrome.com/docs/extensions/reference/manifest/key) / [打包准备](https://developer.chrome.com/docs/webstore/prepare) | `key` 用于固定开发加载的 ID；官方流程先上传草稿，再从 Package → View public key 获取公钥供本地使用，并比较 ID。ZIP 中 manifest 应位于根目录。本次实际上传错误为 `key field is not allowed in manifest`，因此商店包省略 `key`，不把本地固定公钥当成商店身份。 |
| [Native Messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging) | 原生 Host 单独注册，`allowed_origins` 必须明确允许扩展来源；这也是首次上架必须检查商店 ID 的原因。 |

## 官方页面差异

商店介绍概览把视频列在“must provide … except Marquee”列表中，而专门的图片规范明确只把图标、小宣传图与截图列为必需。本套按专项图片规范准备必需图片；视频作为可选项保留，提交时再核对实际后台。没有访问发布者后台，不能声称其字段、类别和校验均已通过。

## 本地事实来源

| 事实 | 来源 |
| --- | --- |
| MV3、版本、必需与可选权限、CSP | 本地：`browser/.output/chrome-mv3/manifest.json`，生成源 `browser/wxt.config.ts`；商店：`browser/.output/chrome-mv3-store/manifest.json`，配置 `browser/wxt.store.config.ts`；以最终 ZIP 为准 |
| 中英文包元数据、短描述 | `browser/public/_locales/en/messages.json`、`browser/public/_locales/zh_CN/messages.json` |
| 当前网站匹配、关联、原生请求及用户触发填充 | `browser/entrypoints/background.ts`、`browser/src/lib/model.ts`、`browser/src/lib/fill.ts` |
| 编辑表单内生成器入口、自定义字段维护和详情展示范围 | `browser/src/components/EditorForm.tsx`、`browser/src/components/Generator.tsx`、`browser/src/components/App.tsx` |
| `otp` 字段生成 TOTP 并交给扩展展示/填充 | `client/vault/model.go` 的 `ParseDocument` / `TOTP`、`client/app/service.go` 的 `dataDocument`、`browser/entrypoints/background.ts` |
| 本地扩展偏好 | `browser/src/lib/appearance.ts`、`browser/src/lib/language.ts`、`browser/src/lib/popup-shortcuts.ts`、`browser/src/lib/page-shortcuts.ts` |
| 会话失效及界面清理 | `browser/src/lib/session.ts`、`browser/src/components/App.tsx` |
| 原生传输、本机端点、固定 Chromium ID | `client/nativehost/host.go`、`client/nativehost/registration.go` |
| 密钥创建/加密、恢复记录与 Git 范围 | `client/vault/vault.go`、`client/vault/crypto.go`、`client/vault/recovery.go`、`client/vault/git.go` |
| 默认设置、活动记录与保留规则 | `client/app/service.go`、`client/app/types.go`、`client/app/activity.go`、`client/app/git.go` |
| 主页与支持链接 | 仓库 origin：`https://github.com/knothhe/passpine.git` |

源码路径用于维护者审计。与用户公开的政策应按其说明单独发布，不能要求用户阅读源码才能了解数据用途。
