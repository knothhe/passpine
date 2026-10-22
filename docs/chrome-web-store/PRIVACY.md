# Passpine 隐私政策 / Privacy Policy

最后更新 / Last updated: 2026-10-22

适用版本 / Reviewed version: Passpine browser extension 0.1.2 and its companion desktop client

> 发布准备说明（供维护者，公开发布时移除此段）：此政策尚未发布为经验证可匿名访问的 URL。发布者需确认本政策与实际运营一致，在下方“联系”补充已启用的私密联系渠道，再将公开 URL 填入商店后台。不得填写不存在或未发布的链接。

## 简体中文

### 用途与范围

Passpine 通过本机桌面客户端管理兼容 pass 的密码库。浏览器扩展让你搜索和编辑账号、复制凭证、生成密码，并按你的操作填写当前网页的登录字段。使用扩展需要安装并运行桌面客户端。

### 处理的数据及用途

Passpine 为提供上述功能处理以下信息：

- 密码库名称、条目名称、目录及锁定状态，用于显示账号列表、查找和管理条目。客户端默认允许锁定时浏览条目名称，你可以在客户端设置中关闭。
- 你保存的用户名、密码、网站地址、一次性验证码相关字段、备注及自定义字段，用于管理所选账号。详情页展示用户名、密码、网站、备注和可用的验证码；自定义字段可在编辑表单中维护。复制和网页填充针对用户名、密码或验证码。你在自由文本中保存的内容也会作为条目内容处理。
- 当前标签页的 URL 和主机名，用于显示当前网站、匹配账号并确认填充时页面没有变化。触发填充时，扩展检查页面登录输入框的类型、属性、可见性及表单结构。
- 账号与网站的关联决定、语言、外观、字号和快捷键设置，用于保持你的偏好。
- 客户端的本机操作记录，包括时间、操作名称、来源及结果，用于查看近期操作。此记录不保存密码、条目正文或完整网页内容。
- 在原生客户端创建或导入密钥时提供的密钥身份信息、私钥及口令，用于加密和解锁密码库。扩展不会获得独立的私钥会话。系统解锁如已启用，由客户端及操作系统处理。

扩展受检版本未包含广告、分析 SDK 或遥测上传功能，也没有向开发者数据服务器上传密码库或浏览活动的接口。它不会持续记录你访问的所有页面。上述说明仍包括本机数据处理，并不意味着扩展“不处理用户数据”。

### 数据保存与保护

条目内容以 OpenPGP 加密的 `.gpg` 文件保存在你选择的密码库目录。私钥以受口令保护的形式保存在客户端应用数据目录中，与同步的密码库分开。条目路径、文件名、`.gpg-id` 收件人标识及 Git 元数据不因条目内容加密而隐藏。

扩展通过 Native Messaging 与同机程序交换请求；Native Host 通过带令牌验证的本机回环连接转发给桌面客户端，该本机连接不使用 TLS。需要展示或填充的解密内容会短暂存在于扩展运行内存。扩展在隐藏、锁定、断连和切换密码库时清理相关界面状态；这不承诺操作系统或运行时内存中所有副本都会即时物理擦除。

扩展使用浏览器本地存储保存偏好及网站关联，不使用 Chrome 同步存储保存密码。关联以由密码库 ID、条目路径、精确主机名计算的 SHA-256 键及布尔值保存；哈希不是加密或匿名化保证。完整当前 URL 不作为浏览历史持久保存。偏好及关联没有自动过期时间。

客户端操作记录默认启用，默认最多保留 7 天和 1,000 条，可调整或关闭；客户端运行时定期清理过期记录，应用停止运行期间不会执行清理。

### 数据交给谁

查看、编辑等操作在你的浏览器与同机客户端之间完成。你要求填充时，所选用户名、密码或验证码会写入当前网页，网页脚本可以读取并按网站自己的逻辑处理这些值。Passpine 不会替你提交登录表单，但网页自身行为由该网站控制。HTTP 网页没有 HTTPS 的传输保护，请先核对网站。

你要求复制时，内容会进入操作系统剪贴板，其他有权限的软件或剪贴板历史功能可能访问。凭证复制由客户端处理，默认 30 秒后仅在剪贴板内容仍为该值时清空；Passpine 无法清除其他软件已保存的副本。复制扩展 ID 仅写入非凭证的扩展标识。

Git 自动同步默认关闭。你配置并启动 Git 同步，或在客户端启用自动同步后，客户端会使用系统 Git 和已有认证配置连接你指定的仓库，传输加密的库文件及可见的文件路径、收件人标识和 Git 元数据。该服务的账户信息、访问日志、保留与删除由你选择的托管方及其政策决定。私钥不放入同步的密码库。

你主动打开项目链接或提交支持请求时，GitHub 等目标服务按其自身政策处理访问及你提供的信息。这与扩展后台自动上传不同。

### 使用限制与删除

Passpine 将用户数据用于上述密码管理功能，不出售用户数据，不用于广告，也不用于评估信用或放贷。从 Google API 获得的信息按 Chrome Web Store User Data Policy（包括 Limited Use 要求）使用。

你可以修改条目、取消网站关联、调整偏好、锁定会话或卸载扩展。删除账号会先移入客户端加密回收站，仍可恢复；需要彻底移除该副本时，在客户端永久删除回收站记录。卸载扩展会移除其本地浏览器数据，但不会删除密码库、客户端应用数据、恢复私钥导出、系统备份、剪贴板历史或 Git 远端。要清除这些数据，请分别在客户端、文件系统、系统密钥存储及你选择的服务中处理。删除工作副本不会自动删除 Git 历史或远端备份。

### 联系与更新

项目维护者通过 [Passpine Issues](https://github.com/knothhe/passpine/issues) 接收公开问题。不要在公开问题中提交密码、私钥、口令、验证码、完整密码库或个人敏感信息。发布者的私密联系渠道：**待发布者补充并验证后生效**。

本政策会随产品数据实践变化而更新，并在上方注明日期。

## English

### Purpose and scope

Passpine manages a pass-compatible password vault through a local desktop client. Its browser extension lets you search and edit accounts, copy credentials, generate passwords, and fill login fields on the current page when requested. The desktop client must be installed and running.

### Data and purposes

To provide these features, Passpine processes:

- Vault and entry names, folders, and lock status for listing and managing entries. By default, the client permits browsing entry names while locked; this can be disabled in its settings.
- Usernames, passwords, website addresses, one-time-code fields, notes, and custom fields you store, to manage selected accounts. Details show the username, password, website, notes, and available one-time code; custom fields can be maintained in the editor. Copying and webpage filling use the username, password, or one-time code. Content you place in free-text fields is also processed as entry content.
- The current tab's URL and hostname to show the website, match accounts, and check that the page has not changed. When you request filling, the extension inspects login inputs' types, attributes, visibility, and form structure.
- Website-association decisions and language, appearance, text-size, and shortcut preferences.
- Local client operation records containing time, action, source, and result. These records do not contain passwords, entry bodies, or full page contents.
- Key identity information, private keys, and passphrases supplied in native client key-creation, import, and unlock flows. The extension does not own an independent private-key session. Optional system authentication is handled by the client and operating system.

The reviewed extension contains no advertising, analytics SDK, or telemetry-upload feature, and no interface that uploads vaults or browsing activity to a developer data server. It does not continuously record all pages you visit. It still handles the local data described here.

### Storage and protection

Entry contents are stored as OpenPGP-encrypted `.gpg` files in your selected vault directory. Passphrase-protected private keys are kept in the client's application data directory, outside the synchronized vault. Entry paths, filenames, `.gpg-id` recipient identifiers, and Git metadata are not hidden by entry encryption.

The extension uses Native Messaging to communicate with a program on the same computer. The Native Host relays requests to the desktop client over a token-authenticated loopback connection without TLS. Decrypted content needed for display or filling temporarily exists in extension memory. Related UI state is cleared when hidden, locked, disconnected, or switched to another vault; this is not a guarantee of immediate physical erasure of every runtime or operating-system memory copy.

Preferences and associations use local browser storage. Passwords are not saved in Chrome sync storage. An association stores a SHA-256 key derived from the vault ID, entry path, and exact hostname, together with a boolean decision. Hashing is not a guarantee of encryption or anonymization. The full current URL is not persistently saved as browsing history. Preferences and associations have no automatic expiry.

Local client operation recording is enabled by default, with default limits of 7 days and 1,000 records. You can adjust or disable recording. Expired records are pruned while the client runs; deletion does not run while it is stopped.

### Recipients

Viewing and editing operate between your browser and local client. When you request filling, the selected username, password, or code is written into the current webpage. That page's scripts can read and process those values under the website's own behavior. Passpine does not submit login forms, but does not control the page's own actions. HTTP pages lack HTTPS transport protection; check the website before filling.

Copying places content on the operating-system clipboard, where other permitted software or clipboard-history features may access it. The client handles credential copying and, by default, clears its value after 30 seconds only if that value has not been replaced. It cannot remove copies already retained elsewhere. Copying the extension ID writes only the non-credential extension identifier.

Automatic Git sync is off by default. If you configure and invoke sync, or enable automatic sync in the client, the client uses system Git and your existing authentication configuration to connect to your chosen repository. It transfers encrypted vault files and visible paths, recipient identifiers, and Git metadata. Your chosen provider controls its account information, access logs, retention, and deletion under its own policy. Private keys are kept outside the synchronized vault.

If you open project links or send a support request, the destination, such as GitHub, handles the visit and information you provide under its own policy.

### Use limitations and deletion

Passpine uses user data for the password-management features described above. It does not sell user data, use it for advertising, or use it to determine creditworthiness or for lending. The use of information received from Google APIs will adhere to the Chrome Web Store User Data Policy, including the Limited Use requirements.

You can edit entries, remove website associations, change preferences, lock the session, or uninstall the extension. Deleting an account first moves it to the client's encrypted trash; permanently delete that record in the client to remove that copy. Uninstalling removes extension-local browser data, but does not remove vault files, client data, exported recovery keys, system backups, clipboard history, or Git remotes. Remove those separately through the client, filesystem, system key storage, and chosen services. Deleting a working copy does not automatically delete Git history or remote backups.

### Contact and changes

Public support is available at [Passpine Issues](https://github.com/knothhe/passpine/issues). Do not post passwords, private keys, passphrases, codes, full vaults, or sensitive personal information there. Publisher's private contact channel: **pending publisher confirmation and activation**.

This policy will be updated when data practices change, with the date shown above.
