# 隐私与权限后台填写参考

版本 0.1.7；核对日期 2026-10-23（初版按 0.1.2 于 2026-10-22 编写）。已读取 0.1.7 Chrome ZIP 内的 `manifest.json`，不是仅依据源配置推测；权限与 0.1.2 相同。0.1.7 新增的客户端通道选择（`clientTarget`）只在开发者解包安装时可选并写入 `storage.local`，商店安装固定连接正式客户端，不写入该值；读取安装类型的 `management.getSelf` 不需要额外权限。此文件供发布者填写后台；面向用户的政策见 [PRIVACY.md](PRIVACY.md)。

## 单一用途

中文：将本机 Passpine 密码库连接到浏览器，供用户查找、管理和复制凭证，并按用户操作填写当前网页的登录字段。

English: Connect the user's local Passpine vault to the browser so they can find, manage, and copy credentials and fill login fields on the current page when requested.

## 权限理由

以下英文可粘贴到对应权限字段。

| 最终包权限 | 可填写的英文理由 | 实现证据 |
| --- | --- | --- |
| `nativeMessaging` | Communicate with the separately installed local Passpine client through the com.knothhe.passpine Native Messaging host. The client owns the vault and unlock session, retrieves and encrypts entries, generates passwords, and performs native clipboard operations. | `entrypoints/background.ts` 的 `connectNative(HOST)`；`src/lib/native.ts`；`client/nativehost/host.go` |
| `activeTab` | Access the active tab after the user invokes Passpine to display its hostname, match accounts, and verify that filling still targets the page the user selected. This avoids requesting persistent access to all websites. | `background.ts` 的 `activeTab`、`currentTab`、`snapshot`；`src/lib/model.ts` 的 `tabContext` |
| `scripting` | Inject the packaged fillPage function into the top frame of the active page only when the user requests filling. It locates visible login fields, writes the selected username, password, or one-time code, and does not submit the form. | `background.ts` 的 `executeScript`；`src/lib/fill.ts` |
| `storage` | Save extension language, appearance, popup keyboard shortcuts, and account-to-website association decisions in local browser storage. Associations use a SHA-256 key derived from the vault ID, entry path, and exact hostname. Passwords and decrypted entry contents are not persisted in extension storage. | `background.ts` 的 `associations`、`setAssociation`；`src/lib/model.ts` 的 `associationKey`；偏好模块 |
| `clipboardWrite` | Let the user copy the extension ID from the connection settings page for setup and troubleshooting. Credential copying uses the local client's clipboard handler so the client's expiry setting can apply. The extension does not read the clipboard. | `components/Options.tsx` 的 `navigator.clipboard.writeText(extensionId)`；`src/lib/client.ts`；`client/app/service.go` 的 `copy`、`copy-generated` |

路径未带 `client/` 前缀的证据均相对 `browser/`。本包未声明 `host_permissions`、`optional_host_permissions`、`optional_permissions`、`tabs`、`history`、`clipboardRead` 或常驻 `content_scripts`。`activeTab` 仍会处理当前网址，不能据此写“完全不访问浏览数据”。

## 远程代码

建议选择 **No, I am not using remote code**，填写：

All executable JavaScript and the injected fillPage function are packaged with the extension. The extension does not download or execute remote scripts, use a CDN for executable resources, or evaluate server-provided code. Its extension-page Content Security Policy sets script-src to self and connect-src to none. Native Messaging connects to a separately installed local companion application; it is not a remote code download service.

已检查入口、最终 manifest 与打包结构；正式上传前仍应对最终 ZIP 重复检查，不能把源配置的检查当成后续任意构建都符合。

## 数据类型建议

Chrome 要求披露本地处理的数据，不能因为开发者没有服务器就直接选择“未处理用户数据”。以下按实际用途给出保守填写建议；后台最终标签与说明由发布者逐项核对。[官方隐私字段](https://developer.chrome.com/docs/webstore/cws-dashboard-privacy)与[本地处理说明](https://developer.chrome.com/docs/webstore/program-policies/user-data-faq)。

| 后台数据类别 | 本包的处理范围 | 填写建议 |
| --- | --- | --- |
| Authentication information | 用户名、密码、OTP/其条目字段；仅在用户请求和有效解锁会话中读取或写入 | 勾选并说明用于密码管理与填充 |
| Personally identifiable information | 用户保存在条目内的用户名、邮箱、备注和自定义字段；原生客户端创建密钥时可使用姓名/邮箱 | 勾选；不等于要求用户注册开发者账户 |
| Web history | 用户调用扩展时的当前标签页 URL/hostname，及从其派生的关联数据 | 勾选并说明仅为当前网页匹配与填充，不持续记录浏览历史 |
| Website content | 用户触发填充时检查当前页面输入框类型、可见性、属性和表单结构，并写入所选值 | 勾选并说明范围限登录填充；不抓取整页正文 |
| User activity | 客户端默认在本机记录操作时间、动作、来源和结果；不包含密码或完整网页内容 | 按配套客户端属于本扩展数据流披露；勾选并注明本机活动记录，默认保留 7 天且最多 1,000 条 |
| Health / Financial / Personal communications / Location | 未实现对应专门采集接口；用户可在自由文本字段主动保存此类内容 | 不把“未主动采集”写成“绝不会处理”；根据实际发布用途和后台定义判断是否需要补充勾选 |

与用户数据用途有关的认证语句可采用以下表述，发布者应先确认其运营实践也成立：

> User data is used only to provide Passpine's disclosed password-management features. It is not sold, used for advertising, or used to determine creditworthiness or for lending. Credentials are shared with a website only when the user requests filling, and encrypted vault files are sent to a Git remote only when the user configures and invokes synchronization or enables automatic sync in the desktop client.

源码未发现广告、分析 SDK、遥测上传或开发者托管的数据接收 API。该结论只覆盖本仓库受检版本，不能替发布者对未披露的运营系统作保证。

## 数据流证据表

| 行为 / 数据 | 触发与默认状态 | 接收方与传输 | 保存及清理 | 证据 |
| --- | --- | --- | --- | --- |
| 库名称、条目路径与目录、锁定状态 | 打开弹窗/设置、刷新与状态检查 | 扩展 ↔ 同机 Native Host ↔ 客户端；原生 Host 使用 stdin/stdout，随后以令牌验证的 loopback TCP 中继到客户端 | 扩展运行内存；原生设置保存库位置。客户端默认允许锁定时浏览名称，可在客户端关闭 | `background.ts`；`client/nativehost/host.go`；`client/app/service.go` 的 `snapshotWithEntries` |
| 密码、用户名、备注、OTP、自定义字段 | 用户查看、编辑或保存；复制/填充限用户名、密码和验证码；需有效解锁会话 | 本机扩展与客户端；填充时仅将所选用户名、密码、验证码交给当前顶层网页，复制时将所选字段交给系统剪贴板 | 账号内容在库中为 `.gpg`；扩展详情在内存，隐藏、锁定、断连、切库时清理状态。页面/剪贴板不是密码库加密存储 | `App.tsx`；`session.ts`；`background.ts`；`client/vault/crypto.go` |
| 私钥与私钥口令 | 客户端创建/导入/解锁 | 留在原生客户端流程，不由扩展建立独立私钥会话 | 受口令保护的私钥位于应用数据目录；解锁密钥在客户端会话内；可选系统解锁由客户端/系统处理 | `client/vault/vault.go`；`client/vault/crypto.go`；`client/app/service.go` 的浏览器请求白名单 |
| 当前网址、表单结构 | 用户调用扩展时匹配；用户填充时读取输入框结构 | 本机浏览器；填充值可被当前网页脚本读取或按网页逻辑处理 | URL 在运行内存；不持久保存完整 URL。`associations` 保存非匿名的哈希派生键及布尔决定，未设自动过期 | `model.ts`；`fill.ts`；`background.ts` |
| 语言、主题、字号、弹窗快捷键 | 用户设置；默认跟随系统/黑白主题 | `browser.storage.local`，不使用 `storage.sync` | 持续保存至修改、清除该扩展数据或卸载 | `language.ts`；`appearance.ts`；`popup-shortcuts.ts` |
| 复制值 | 用户点击复制；凭证复制由客户端处理 | 操作系统剪贴板，其他有权限的软件可能读取 | 客户端默认 30 秒后在值未被替换时清空所复制内容；系统剪贴板历史和其他软件副本不受保证 | `client/app/service.go`；`client/desktop/desktop.go`；`Options.tsx` |
| 操作记录 | 客户端默认开启，可关闭 | 仅本机 `activity.json`，无实现中的上传端点 | 默认 7 天/最多 1,000 条；客户端运行中定期裁剪，关闭时不会后台运行删除 | `client/app/activity.go`；`client/app/service.go` |
| Git 同步 | 默认关闭自动同步；用户配置远端并手动同步，或开启自动同步 | 用户配置的 Git 托管方及其认证工具；客户端执行系统 Git | 仓库保留加密 `.gpg`、`.gpg-id` 及可见路径/Git 元数据；远端保留与删除由用户及托管方控制 | `client/app/git.go`；`client/vault/git.go` |

## 提交前尚需人工核对

1. 发布者在后台采用的所有数据类型、Limited Use 认证必须与实际运营实践一致。
2. 将隐私政策公开并验证匿名访问，补齐可用的私密联系渠道；现有 Issues 链接仅适合不含敏感数据的公开反馈。
3. 本包会在用户选择后向 HTTP 网页填充，界面显示 HTTP 提示，但不会强制 HTTPS。提交时如实保留该限制，结合实际目标网站评估 Chrome 的安全处理规则，不宣称全部传输强制加密。
4. 查看商店分配的 Item ID；若与当前 `obnlebeedcbbchadakacilopfdaifgjh` 不一致，按 [官方 key 流程](https://developer.chrome.com/docs/extensions/reference/manifest/key)同步 manifest key、原生 `ChromiumID` 和 Host `allowed_origins` 后重新构建安装及端到端测试。不能仅修改商店文案。
