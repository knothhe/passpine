# 配置同步

客户端设置 → **配置同步**可连接用户自己的 Cloudflare R2 或 S3 兼容对象存储。密码库继续使用原有 Git 同步，两条通道独立。

首版同步客户端和浏览器扩展的主题风格、明暗外观、字号、语言；两端分别保存，不强制使用同一外观。本机路径、当前密码库、私钥、系统解锁、浏览器权限、快捷键、安全时长、开机自启和操作记录不上传。

## 手动 S3 配置

填写服务端点、存储桶、区域、目录前缀、Access Key ID、Secret Access Key，可选 Session Token 与路径寻址。R2 通常使用 `https://<account>.r2.cloudflarestorage.com`、区域 `auto` 和路径寻址；其他 S3 服务使用其实际区域与端点。端点不包含桶名、查询参数或凭据，仅支持 HTTPS；隔离本地服务可用 loopback HTTP。

“测试连接”只读取桶列表，不创建、修改或删除对象；成功仅表示当前读取权限可用，写入权限在实际同步时验证。“保存并连接”先完成候选连接的首次同步，成功后再替换已保存连接；口令错误、文档损坏或写入失败时保留原连接及编辑草稿。保存后的旧凭据清理失败会保留清理记录与警告，新连接仍然生效。已保存的同一目标可留空秘密输入以保留本机凭据；更换端点或桶需要重新填写凭据。

每个同步空间使用独立的加密口令，至少 12 个字符。在其他设备填写相同的端点、桶、前缀和加密口令即可恢复。此口令与密码库的私钥口令无关，Passpine 没有服务端恢复功能。手动与 OAuth 连接方式不改变同一空间的身份。

## Cloudflare OAuth 发行配置

发行者在 Cloudflare 注册 **Passpine 自己的** OAuth client；普通用户只需点击授权并选择可访问账号、辖区和已有桶。缺少 Client ID 时授权入口明确不可用，手动 S3 不受影响。

公开配置位于 [`client/r2config/publisher.json`](../client/r2config/publisher.json)，可通过 `PASSPINE_CF_OAUTH_CLIENT_ID` 和 `PASSPINE_CF_OAUTH_REDIRECT_URL` 显式覆盖。仓库不包含其他应用的 Client ID 或 Client Secret。当前回调地址：

```text
http://127.0.0.1:47839/oauth/callback
```

注册必须精确匹配此地址。Grant type 使用 `authorization_code` 和 `refresh_token`，Response type 使用 `code`，Token endpoint authentication 使用 `none`，PKCE 使用 `S256`。所需业务权限以已锁定版本的 [`r2connect.RequiredScopes()`](https://pkg.go.dev/github.com/knothhe/r2connect#RequiredScopes) 为准；SDK 统一请求 `offline_access`，Passpine 不另维护一份 scope 规则。

同意页如果将 **Account API Tokens Write** 放在 Sensitive scopes，需要在 Edit Permissions 中开启。账号需要具备创建专用 Account API Token 的权限。应用只派生所选桶的对象权限，不创建桶，不申请整个账号的 R2 管理权限。

先用 private visibility 和发行者测试账号验证；对其他账号开放前，按 Cloudflare 要求配置应用名称、Logo、Client URL、scopes 和域名验证，再决定是否改为 public visibility。仓库开发不会自动注册 OAuth client 或更改其可见性。

参考：[OAuth 注册](https://developers.cloudflare.com/fundamentals/oauth/create-an-oauth-client/)、[OAuth 集成](https://developers.cloudflare.com/fundamentals/oauth/integrate-with-cloudflare/)、[R2 凭据派生](https://developers.cloudflare.com/r2/api/tokens/)。

## SDK 与平台构建

Cloudflare OAuth、loopback 回调、账号和存储桶 API、桶凭据派生、续期与清理由 [`github.com/knothhe/r2connect`](https://github.com/knothhe/r2connect) 统一实现，版本锁定在 `client/go.mod`。Passpine 保留自己的公开发行配置、同步空间和加密对象引擎；OAuth 与手动连接最终都使用同一套 S3 数据读写。

macOS 的 SDK Keychain 后端需要 cgo。`scripts/build-client.sh` 在执行构建工具前检查并导出 `CGO_ENABLED=1`，显式禁用 cgo 会报错停止；需要 Xcode Command Line Tools。现有 macOS Release CI 已设置 `CGO_ENABLED=1`。Windows 使用原生 Credential Manager，Linux 使用 Secret Service；不因 macOS 的构建要求改写另外两端的编译标志，也不以交叉编译结果代替系统凭据的原生验收。

授权没有返回刷新令牌时仍可保存有效桶凭据，界面会提示此授权无法静默续期，应在桶凭据到期前重新连接。SDK 清理旧 Token 或授权失败时会保留归属记录，并持续显示清理提示；不能把授权或同步成功当作旧凭据已经撤销。

SDK 元数据读取失败单独显示，保留已保存目标和编辑输入，不能降级成“未连接”后覆盖原文件。修复配置目录中的有效状态后可点击“重新读取配置”；此操作只重建读取失败的 SDK manager，不读取系统凭据，也不重建健康 manager 或丢弃其尚未持久化的旋转令牌。

## 数据、安全与冲突

对象默认保存在 `passpine/settings.v1.enc`。同步文档有版本和命名空间，当前使用 `desktop.*` 与 `browser.*`；未知字段在读写时保留，为后续数据类型留出空间。新增数据仍需显式定义本机持久化、权限与合并规则，不能直接序列化整个应用设置或密码库。

同步内容在客户端通过 scrypt 派生密钥、AES-GCM 认证加密后上传。访问密钥、OAuth access/refresh token、派生桶凭据、加密口令与客户端合并基线只保存在本机系统凭据存储：macOS Keychain、Windows Credential Manager、Linux Secret Service。Dev、正式版和不同数据目录使用独立命名空间；系统存储不可用时不会退回明文文件。取消或拒绝访问后停止本次运行的自动凭据访问，用户主动连接或断开时才能重试。

扩展通过本机客户端同步，不接收云端凭据。扩展本地存储分别记录各客户端及同步空间的外观、语言基线和待上传修改，后台重启、离线或暂停期间的修改在恢复连接后继续发送；切换客户端或同步空间不会把另一目标的待上传设置混入新目标。

首次连接优先采用远端已有字段；之后按字段进行三方合并。不同字段的修改可合并，同字段出现不同修改会暂停同步并显示冲突，用户选择保留本机或使用远端后重试。提交使用 ETag 条件写入，避免并发上传静默覆盖。错误口令、损坏文档和未来版本不会被当作空文档覆盖。

同步按周期检查，保存本地偏好后安排合并；失败退避重试，冲突等待用户处理。暂停保留连接和远端对象；断开清理本机连接，并撤销应用创建的 R2 桶 Token 和 OAuth 授权，保留用户桶内数据。手动填写的密钥由用户管理，不自动撤销。

## 验证范围

自动测试使用临时目录、内存凭据存储和本地 HTTP 服务，覆盖加密、S3 签名、并发条件写入、字段合并、重启基线、OAuth 回调与权限校验、凭据访问拒绝、续期与清理。不会读取真实密码库、访问真实桶或修改用户系统凭据。

真实 OAuth 验收需要发行者自己的 Client ID、隔离应用数据目录与专用测试桶；真实系统凭据授权及 Windows/Linux 原生行为需要在目标系统验证。交叉编译或本地替身测试不替代这些验收。
