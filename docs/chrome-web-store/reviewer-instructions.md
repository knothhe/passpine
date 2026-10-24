# 审核测试说明 / Reviewer instructions

对应版本：0.1.7。商店截图已用 0.1.7 真实生产扩展的工具栏弹窗，通过真实 Native Messaging 连接隔离 Go 客户端重拍，中英文账号列表、详情及生成器各 3 张，全部使用虚构数据，见 [素材记录](assets.md)。以下是从干净安装开始的完整审核步骤；这些实拍只证明其覆盖的场景。

## 发布者提交前补齐

- 桌面客户端下载已填入下方英文说明，指向已发布的 [v0.1.7 Release](https://github.com/knothhe/passpine/releases/tag/v0.1.7)（2026-10-23 核对为正式版、非草稿）。提交更新版本的扩展时，同步改为对应版本的 Release 和文件名。
- 核对商店 Item ID、实际安装 ID、客户端 Native Host `allowed_origins` 一致。当前仓库开发 ID 为 `obnlebeedcbbchadakacilopfdaifgjh`，Host 为 `com.knothhe.passpine`。见 [官方 key 文档](https://developer.chrome.com/docs/extensions/reference/manifest/key)。
- 使用最终商店分配的扩展 ID、最终 ZIP 和正式分发的桌面安装包，复跑以下步骤并记录平台、Chrome 版本、日期与结果。这组完整安装、填充、复制、锁定和恢复路径仍待验收；已完成的实拍与 Native Messaging 连通不能替代完整流程。下方英文说明会粘贴到后台，不在其中写入发布者自己的验收状态。
- 本产品不需要开发者账户、云 API Key、付费服务或现有 password-store。审核人员应创建自己的临时库；本文件中的虚构口令只供临时验收，不用于真实账号。

## 中文测试步骤

1. 按下方英文说明中的地址下载并安装 v0.1.7 桌面客户端，然后运行。在首次设置中选择“创建密码库”，选择一个新的空目录，设置密码库名称及至少 12 个字符的私钥口令，例如 `Passpine-review-2026!`，填写“确认口令”，点击“创建密钥与密码库”。无需预装 GnuPG 或导入密钥。预期：客户端生成受口令保护的 OpenPGP 私钥并进入空密码库。按创建后的提示，在客户端“设置 → 密码库”使用“导出恢复私钥”和“验证恢复备份”；临时测试结束后删除这组测试备份。
2. 在客户端“设置 → 浏览器”确认 Chrome 显示“已注册”；未注册时点击“注册”，已有失效连接时点击“修复”。保持客户端运行。安装审核中的扩展包，打开其页脚“设置与连接”图标，在浏览器设置页点击“检查连接”。预期：显示连接状态、库名称、Host 与扩展 ID。客户端完全退出时显示未连接状态及连接指引；仅关闭窗口可能仍在后台运行。
3. 打开扩展，点击当前网站区域的加号“新建账号”。名称填 `review/local-test`，用户名填 `reviewer@example.invalid`，密码填 `Fixture-password-2026!`，网站填测试页的地址。需要测试 TOTP 时，在“自定义字段 → 添加字段”中将字段名称填为 `otp`，值填为虚构 Base32 种子 `JBSWY3DPEHPK3PXP`。点击“保存”。预期：账号加密保存并出现在客户端和扩展列表中，搜索 `local-test` 能找到它。编辑表单可维护自定义字段；详情页不逐项展示全部自定义字段。
4. 点击账号行右侧的详情箭头“查看 local-test”，切换显示/隐藏密码，复制用户名或密码；在可信的临时文本编辑器内验证复制值。预期：密码默认遮罩；有上一步的有效 `otp` 字段时，详情显示计算出的一次性验证码及倒计时。复制成功后按客户端超时设置清理剪贴板，默认 30 秒，前提是剪贴板值未被其他内容替换。不要将测试凭证粘贴到外部服务。
5. 打开你控制的测试登录页面（可使用下方静态表单），再打开扩展并点击账号正文，或在账号详情页点击“填充账号与密码”。预期：当前顶层页面的用户名和密码输入框被填写，有 `autocomplete="one-time-code"` 时可填写 OTP；扩展不会提交表单。成功后弹窗关闭，并保存精确主机名的关联。重新打开扩展可优先找到该账号；在更多菜单中取消关联后该手动关联不再生效。
6. 从账号详情页右上角点击“编辑账号”，更改并保存，再刷新客户端确认一致。再次进入详情页，点击“删除账号”并确认“移入回收站”。预期：账号从列表移除，在客户端“恢复中心 → 回收站”可恢复；恢复后重新打开扩展可见。
7. 点击“新建账号”，或进入现有账号的“编辑账号”，在密码输入框下点击“重新生成密码”进入“密码生成器”。改变长度及字符类型并重新生成。预期：产生符合选项的密码，可点击“复制”，或点击“使用此密码”将结果带回当前账号表单；保存账号后才写入密码库。不应允许所有字符类型都关闭。
8. 在扩展中锁定密码库，再请求查看或填充。预期：无法直接读取秘密；已配置系统解锁的平台会请求系统验证，否则前往客户端解锁后再继续。不要要求审核人员使用开发者的系统账户或生物识别信息。
9. 在扩展设置切换 English / 简体中文 / 跟随系统、明暗主题与字号。预期：界面即时更新，账号名称与内容不被翻译。关于页面显示 0.1.7。

填充仅支持普通 HTTP/HTTPS 顶层页面；浏览器内部页、跨域 iframe、封闭 Shadow DOM 和部分定制输入框不在本次基本测试范围。HTTP 测试仅用虚构数据。Git 同步属于用户可选的客户端功能，基础审核不需要 Git 账户或远端仓库。

## English instructions for the dashboard

Passpine is a companion to the separately installed local desktop client. No developer account, cloud API key, subscription, existing password-store, or preinstalled GnuPG is required for the basic review.

**Desktop client download (v0.1.7):** https://github.com/knothhe/passpine/releases/tag/v0.1.7

- Windows x64: `Passpine-0.1.7-windows-amd64-setup.exe` — https://github.com/knothhe/passpine/releases/download/v0.1.7/Passpine-0.1.7-windows-amd64-setup.exe . The installer is not Authenticode signed; if SmartScreen appears, choose **More info → Run anyway**.
- macOS Apple Silicon: `Passpine-0.1.7-macos-arm64.zip` — https://github.com/knothhe/passpine/releases/download/v0.1.7/Passpine-0.1.7-macos-arm64.zip . Unzip and move Passpine.app to Applications. The app is not notarized; if macOS blocks the first launch, choose **Open Anyway** in System Settings → Privacy & Security. Intel Macs are not supported by this release.
- Linux x86-64: `Passpine-0.1.7-linux-amd64.tar.gz` — https://github.com/knothhe/passpine/releases/download/v0.1.7/Passpine-0.1.7-linux-amd64.tar.gz . Requires GTK 3 and WebKitGTK 4.1. Extract into an empty folder and run `./install.sh`, which installs to `~/.local/bin` and adds a desktop menu entry.

1. Install and run the companion client. Choose **Create vault**, select a new empty folder, and enter a vault name and a private-key passphrase of at least 12 characters. `Passpine-review-2026!` is a disposable example. Confirm the passphrase and select **Create key and vault**. The client generates a passphrase-protected OpenPGP private key locally. Export and verify a recovery-key backup through client Settings → Vault as prompted. Use a temporary vault only.
2. In client Settings → Browsers, confirm Chrome shows **Registered**. If needed, select **Register**, or **Repair** if its existing registration is invalid. Keep the client running. Install the extension under review. Open the footer's **Settings and connection** icon and use **Check connection** on the Browser settings page. The Native Host is `com.knothhe.passpine`. The publisher must confirm the installed store ID is allowed by the client. Fully quit the client to test disconnection; closing its window may leave it running.
3. Click the plus icon labeled **New account** in the extension's current-website area. Create `review/local-test` with username `reviewer@example.invalid`, password `Fixture-password-2026!`, and your controlled test-page URL. To test TOTP, choose **Custom fields → Add field**, enter `otp` as the field name and the dummy Base32 seed `JBSWY3DPEHPK3PXP` as its value. Save and search for the account. The editor manages custom fields; the details view does not list every custom field.
4. Click the account row's right-hand details arrow labeled **View local-test**. Check password reveal/hide and copying. A valid `otp` field produces a one-time code with a countdown in the details view. Paste copied credentials only into a trusted local temporary editor. Credential clipboard expiry defaults to 30 seconds and clears only if the copied value has not been replaced.
5. Open a controlled page with username and password inputs, invoke the extension, and click the account row, or choose **Fill login** in account details. The top-level inputs should be filled; a supported one-time-code input can also be filled. Passpine does not submit the form. On success the popup closes and links the account to the exact hostname. Reopen the popup to verify the association, then unlink it through the account menu.
6. In account details choose **Edit account**, save a change, and verify it in the client. Reopen details, choose **Delete account**, confirm **Move to trash**, then restore it through the client's Recovery → Trash.
7. Open **New account** or **Edit account**, then select **Regenerate password** below the password input to open **Password generator**. Change length and character groups and regenerate. Select **Copy**, or **Use this password** to return the result to the current account form. Saving the account writes it to the vault.
8. Lock the vault and request a credential. Secrets require another valid unlock. Configured supported devices may prompt for system authentication; otherwise unlock in the desktop client. Reviewers do not need the publisher's operating-system credentials.
9. Switch English / Simplified Chinese / System default, color mode, and text size in extension settings. UI labels change while user entry content remains unchanged. The About page shows version 0.1.7.

The extension cannot operate as a standalone vault. Browser internal pages, cross-origin frames, closed Shadow DOM, and some custom forms are not supported by the basic fill path. Git synchronization is optional and is not needed for review.

## 可选本地测试表单 / Optional local fixture

保存下面内容为 `login.html`，放在一个只包含此虚构测试文件的临时目录，在该目录执行 `python3 -m http.server 8787 --bind 127.0.0.1`，再打开 `http://127.0.0.1:8787/login.html`。表单没有提交按钮、脚本或外部资源；停止服务器并删除临时目录即可清理。不要从个人文件目录启动静态服务器。

Save this as `login.html` in a dedicated temporary folder containing no personal files. Run `python3 -m http.server 8787 --bind 127.0.0.1` there and open `http://127.0.0.1:8787/login.html`. Stop the server and remove the folder afterward. It requires Python 3; you can instead use any controlled HTTP/HTTPS login page without signing in.

```html
<!doctype html>
<html lang="en">
<meta charset="utf-8">
<title>Passpine local review fixture</title>
<h1>Local review fixture — fictitious credentials only</h1>
<form>
  <p><label>Username <input name="username" autocomplete="username"></label></p>
  <p><label>Password <input name="password" type="password" autocomplete="current-password"></label></p>
  <p><label>One-time code <input name="otp" autocomplete="one-time-code"></label></p>
</form>
</html>
```

审核测试不要求把真实密码库、私钥、口令或系统解锁凭证交给发布者。若后续版本增加私有服务，账号、端点、专用测试 Key、有效期等只能通过后台 Test instructions 的适当私密渠道提供，不能写入公开材料。
