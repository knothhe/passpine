# 客户端更新

PassPine 正式版默认在启动后于后台检查更新，之后每 6 小时检查一次。默认自动下载；设置中关闭「自动下载更新」后仍自动检查，用户可手动下载。下载不会关闭密码库、退出应用或重置正在编辑的内容。下载完成只提示待安装，必须由用户点击确认重启并安装。源码构建及本地 `make build` / `make install` 使用隔离的 Dev 通道，不检查或安装正式更新。

设置页展示当前版本、候选版本，提供自动下载开关与「检查更新」，覆盖检查中、已是最新、发现更新、下载进度、待安装与失败重试。更新说明使用整条可点击的链接行，打开对应的 GitHub Release，不在客户端内嵌正文。链接优先使用有效的稳定候选版本，否则使用当前稳定版本；两者均没有稳定版本时禁用入口。链接固定指向项目仓库的 `releases/tag/v<版本>`。应用文案沿用 `client/i18n` 中英文目录。

## 信任与启用

更新源默认是 `https://github.com/knothhe/passpine/releases/latest/download/update-feed.json`。HTTPS 只负责传输，信任来自编译进客户端的 Ed25519 公钥。Feed 内完整包的 URL、版本、大小、SHA-256 和目标文件清单都包含在签名中。客户端下载并校验 Feed 后才信任包的地址和哈希。更新说明正文由 GitHub Release 展示，不写入 Feed。

生产信任根固定在仓库的 `config/update-signing.json`，只包含公钥、算法、仓库和 Feed 地址。正式 CI 构建默认嵌入这个公钥，发布时要求仓库、Feed 和公钥与固定配置一致。仓库配置如下：

| GitHub 配置 | 格式及用途 |
| --- | --- |
| Repository variable `PASSPINE_UPDATE_PUBLIC_KEY` | 标准 Base64 编码的 32 字节 Ed25519 公钥，须与仓库固定公钥一致；为空时使用固定值，无需保密 |
| Repository secret `PASSPINE_UPDATE_PRIVATE_KEY` | 对应的 Ed25519 PKCS#8 PEM 私钥，只交给 Release 的 publication job |
| Repository variable `PASSPINE_UPDATE_FEED_URL` | 上述固定的 GitHub HTTPS Feed 地址；为空时使用仓库固定值 |

生产私钥存入 CI Secret，不能放入仓库、公开变量、构建产物或日志。公钥是公开的验证材料，保存在仓库和 Actions Variable 中；私钥仅提供给发布 job，不进入任何构建 job。发布脚本从私钥推导公钥并要求与固定公钥完全一致；缺少私钥、密钥不匹配或配置偏离均停止发布，不能降级为无签名发布。测试使用运行时临时生成的测试密钥。

当前固定公钥的原始 32 字节 SHA-256 为 `4d7c57c7719d09e3ee65db7b6e214b37174e7fe17098d3fe980dc222b225a73c`。私钥的本地备份位于仓库外的 `~/.config/passpine-release/update-signing/update-ed25519-v1.pkcs8.pem`，目录权限 `0700`、文件 `0600`；应另行保管受控离线备份。不要重新生成同名密钥来修复 CI 配置，这会让已安装客户端无法验证更新。

正式版本仅由 GitHub Actions 对已验证的发布 tag 构建，CI 显式使用 `PASSPINE_BUILD_CHANNEL=release`。本地构建默认 Dev，即使提供更新环境变量也不会启用正式更新。客户端在运行时还检查通道，避免 Dev 误用正式 Feed 或安装助手。首次启用后，旧版若没有嵌入公钥，需要手动安装一次新版正式客户端；网络不能向旧版补发信任根。公钥轮换需要规划兼容迁移。

## 制品与全量下载

沿用现有 tag Release 流程，不新增发版入口，也不改变普通安装包。各平台构建 job 生成专用更新 ZIP：

| 平台 | 完整更新 ZIP | 包内路径 |
| --- | --- | --- |
| macOS arm64 | `Passpine-<版本>-update-darwin-arm64.zip` | 应用 bundle 的 `Contents/...`，无 `Passpine.app` 外层目录 |
| Linux amd64 | `Passpine-<版本>-update-linux-amd64.zip` | `passpine`、`passpine-host` |
| Windows amd64 | `Passpine-<版本>-update-windows-amd64.zip` | `passpine.exe`、`passpine-host.exe` |

ZIP 为标准 ZIP32 / deflate，路径为 UTF-8，无符号链接和目录条目，每个文件使用 `0755` 或 `0644` 权限。macOS 保留完整 bundle 资源及签名。Linux / Windows 更新包只替换两个受控程序文件，不调用普通安装器，也不覆盖卸载器、浏览器注册或相邻文件。

每个平台的 `manifest-client-*.json` 在现有 tag / HEAD 来源记录中增加完整更新包哈希及目标文件清单。publication job 收齐三个平台并重新校验后，只签署当前版本的完整包信息，不查询或下载旧版 Feed，不生成差分包。客户端始终下载完整更新包，下载或验证失败时提供重试。

三个完整更新包和 `update-feed.json` 与五个普通安装附件一起上传同一个 Release 草稿，所有九个确切附件及 SHA-256 核对后才公开，继续遵循已有不覆盖同名制品和失败续推规则。Feed 的签名负载是精确原始 JSON 字节：

```json
{"payload":"<base64 JSON bytes>","signature":"<base64 Ed25519 signature>"}
```

解码后的 payload：

```json
{
  "schema": 1,
  "version": "0.1.4",
  "assets": [{
    "os": "darwin",
    "arch": "arm64",
    "full": {"url": "https://example.com/full.zip", "sha256": "<64 lowercase hex>", "size": 12345},
    "files": [{"path": "Contents/MacOS/passpine", "sha256": "<64 lowercase hex>", "size": 1234, "mode": 493}]
  }]
}
```

`assets` 实际必须包含三个受支持平台，`files` 必须包含该平台的全部目标文件和两个主程序；示例省略其余平台和文件。`os` 使用 Go 平台名 `darwin`、`linux`、`windows`，`arch` 使用 `arm64`、`amd64`。

协议继续使用 `schema: 1`。新 Feed 仅包含 `schema`、`version`、`assets`，每个平台仅包含 `os`、`arch`、`full`、`files`；不要求更换公钥。读取历史已签名 Feed 时仍验证全部原始负载字节，但忽略旧 `notes`、`deltas` 等额外字段，始终使用 `full`。既有 Release 和历史说明文件不会被源码修改追溯改写。

## 安装、恢复与数据保护

下载缓存位于应用数据目录的 `updates/`，权限为 `0700`。缓存记录保存原始已签名 Feed，应用重启后仍须验证签名、完整 ZIP 和目标文件哈希才能恢复「待安装」，不能仅凭缓存状态信任文件。

用户确认安装后，再次验证 Ed25519 签名、ZIP SHA-256、每个目标文件的大小与 SHA-256，以及安装路径和密码库路径互不覆盖。macOS 额外运行 `codesign --verify --deep --strict`，其现有 ad-hoc 签名只验证 bundle 完整性，发布者身份仍由 Feed 的 Ed25519 信任根确认。

独立安装辅助进程等待原客户端退出后执行替换。macOS 在同目录预备新 bundle，通过 macOS 原子交换替换旧 bundle，并保留旧 bundle 作为备份；Linux / Windows 为两个程序分别保留备份后原子替换，并在替换失败时按逆序回滚。安装动作不迁移或改写密码库与设置文件。启动新版本后需在 30 秒内报告启动健康状态，未确认则结束新进程、回滚并重启旧版；失败时保留可恢复备份与用户数据。下载或验证失败只影响缓存，不改写当前安装。

此流程不提供操作系统级断电事务，也不能恢复程序未来主动造成的密码库格式不兼容；涉及数据格式升级时需另行提供可回退的数据迁移设计。当前更新安装阶段本身不操作用户数据。

## 验证与发布边界

`node --test scripts/update-artifacts.test.mjs scripts/ci-release.test.mjs` 覆盖公钥配置、精确负载签名、签名和哈希篡改、密钥不匹配、平台路径与符号链接拒绝、ZIP 互操作、全量输出与确定性、旧协议额外字段兼容、三平台制品来源和九个发布附件的完整性，以及 CI 私钥隔离。客户端的 `go test ./update` 覆盖状态机、完整包下载、失败重试、缓存与安装验证。

配置和本地测试不会推送 tag、发布 GitHub Release、下载生产更新或安装当前应用。生产启用需要提交配置和实现，再依照 [版本管理与发布](versioning.md) 发布包含公钥的新安装包；已有 Release 不会被这些源码改动追溯更新。跨平台真实安装与回滚还应在对应的目标系统做发布验收。

### macOS 实际安装验收

完整包安装、新版崩溃回滚、启动确认超时回滚、macOS 签名失败和安装前包篡改拒绝的实际验证方式与执行结果，见[macOS 隔离验收记录与重现命令](validation/macos-updates.md)。测试使用临时构建和虚构密码库、密钥及偏好文件，不对当前已安装应用执行安装。
