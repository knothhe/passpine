# Passpine 官网

独立的静态官网，使用 pnpm、Vite、React、TypeScript、Tailwind CSS、shadcn/ui 和 Motion。产物位于 `website/dist/`，使用 Cloudflare Workers Static Assets 托管。

## 本地开发与检查

需要 Node.js 22.12+、pnpm 12.11.2。在仓库根目录执行：

```sh
make website:deps        # 根据 pnpm-lock.yaml 安装依赖
make website:dev         # 开发服务器
make website:check       # 类型检查、测试、生产构建
make website:build       # 仅生产构建
make website:preview     # 构建后启动本地预览（Vite，不处理 _redirects）
make website:sync        # 从 GitHub 更新已验证的 Release 快照
```

也可直接使用子项目命令：

```sh
pnpm --dir website install --frozen-lockfile
pnpm --dir website run dev
pnpm --dir website run typecheck
pnpm --dir website run test
pnpm --dir website run build
pnpm --dir website run preview
pnpm --dir website run sync:releases
```

根目录提供 `pnpm run dev:website`、`build:website`、`test:website`、`preview:website`、`sync:website` 和 `deploy:website` 别名。网站版本和部署独立于桌面客户端、浏览器扩展的 SemVer 发布；默认 `make` 不构建或部署官网。常规开发、检查与构建使用仓库中的 Release 快照，不依赖 GitHub 网络访问。

## 页面与路由

每个页面都是独立的静态 HTML 文档，不使用前端路由，也不启用单页应用回退：

| 路径 | 文档 | 入口 |
| --- | --- | --- |
| `/` | `index.html` | `src/entries/home.tsx` |
| `/privacy` | `privacy.html` | `src/entries/privacy.tsx` |
| 其他不存在的路径 | `404.html`（返回 404 状态） | `src/entries/not-found.tsx` |

页面清单、首页区块锚点和隐私政策章节 ID 集中在 `src/constants/routes.ts`。页眉和页脚的区块链接统一写成 `/#features` 这类绝对路径，在任意页面都能回到首页对应位置。`public/_redirects` 提供 `/privacy-policy` → `/privacy` 和 `/download` → `/#download` 两个别名。新增页面时：在 `pageDocuments` 登记、新建同名 HTML 文档和入口、在 `copy.ts` 的 `meta` 中补齐两种语言的标题与描述。

`build` 在 Vite 构建后运行 `scripts/prerender.mjs`，把每个页面以默认语言（简体中文，与文档的 `lang` 一致）渲染进 HTML，使未执行 JavaScript 的爬虫、商店审核工具和访客也能读到正文。浏览器加载后按访客语言重新渲染，并重新定位 URL 中的锚点。

## 隐私政策

`/privacy` 是浏览器扩展、桌面客户端和官网共用的公开隐私政策，可直接填入 Chrome Web Store 后台。正文位于 `copy.ts` 的 `privacy`，最后更新日期和核对所依据的版本位于 `routes.ts` 的 `privacyPolicy`。权限、存储、联网或第三方服务发生变化时，同步修改两种语言的正文并更新日期；测试会检查两种语言的章节结构一致，以及权限表与扩展声明的权限相符。

## 语言

官网支持 `system`、`en` 和 `zh-CN` 三种语言选项。默认跟随系统，不支持的系统语言回退为 English，与客户端和扩展保持一致；手动选择会保存在浏览器中。文案集中管理在 `src/constants/copy.ts`，新增用户可见内容需同步补齐中英文。

## 宣传片

视频区块按当前语言播放中文或英文宣传片。视频和封面不随站点打包，存放在 R2 bucket `oss`（自定义域名 `oss.knothhe.com`）的 `passpine/promo/<版本>/` 下，地址集中在 `src/constants/media.ts`；`public/_headers` 的 CSP 已为该域名放行 `img-src` 与 `media-src`。

宣传片源码位于 `design/promo-video`：`pnpm --dir design/promo-video run render` 渲染中英文成片和封面，登录 Wrangler 后 `pnpm --dir design/promo-video run upload` 上传。已上传的版本不覆盖；更新视频时提高上传脚本中的 `VERSION`，并同步修改 `media.ts`。

## GitHub Releases 下载

安装包来自 [knothhe/passpine 的 GitHub Releases](https://github.com/knothhe/passpine/releases/latest)。下载区按 Release 实际附件生成卡片，仅显示已发布且符合预期命名与下载地址规则的文件，不为尚未发布的系统或架构生成按钮。页面在浏览器中检测操作系统和可用的架构信息，优先排列匹配的安装包；其他实际可下载的文件仍保留在列表中。

浏览器不一定能可靠识别 CPU 架构，尤其是 Apple Silicon 上使用兼容模式时；推荐结果不能作为硬件保证，文件卡片会标明对应的平台和架构。不支持的系统与移动设备可浏览已有文件或前往 GitHub Releases，不会收到不匹配的桌面安装包推荐。

页面先使用随站点构建的已验证 Release 快照，并尝试向 GitHub 获取最新公开 Release。网络不可用、API 限流或返回的数据无效时保留快照中的实际附件，并标明所显示的版本；快照不代表实时的最新版本。访问页面不需要 GitHub Token。

快照存放在 `src/data/release.json`，通过与在线数据相同的校验规则读取。`make website:sync` 运行 `scripts/sync-release.mjs`，仅在获取并验证成功后更新快照。网络失败、GitHub 限流或数据无效时同步会报错，并保留原文件；部署命令会在构建前运行相同同步步骤，同步失败时终止部署。

同步脚本优先使用环境变量 `GH_TOKEN`，其次使用 `GITHUB_TOKEN`；未设置 Token 时尝试通过本机已登录的 GitHub CLI（`gh api --hostname github.com`）请求，CLI 不可用或请求失败才使用匿名 API。匿名请求容易受到共享出口 IP 的限流，遇到 HTTP 403/429 时可先执行 `gh auth login --hostname github.com`，或在部署环境中配置仅有公开仓库读取权限的 Token。错误会显示 HTTP 状态码，以及 GitHub 提供的限流重置时间和重试间隔；显式配置的 Token 无效时直接报错。Token 仅用于 Node 同步进程，不写入快照或前端构建产物。

安装后的平台依赖、签名限制和扩展加载步骤以[本地安装文档](../docs/install-local.md)为准。

## 部署到 Cloudflare

`wrangler.jsonc` 配置静态资源目录 `./dist`，Worker 名称为 `passpine-website`，默认使用账户的 `workers.dev` 域名。这里不预设真实域名。`html_handling` 将 `privacy.html` 发布在 `/privacy`（`/privacy.html` 与 `/privacy/` 会重定向到该地址），`not_found_handling: "404-page"` 让不存在的路径返回 `404.html` 和 404 状态，而不是回退到首页。

首次在本地部署：

```sh
make website:deps
pnpm --dir website exec wrangler login
make website:deploy
```

或直接执行：

```sh
pnpm --dir website run deploy
```

部署命令先从 GitHub 同步并验证 Release 快照，再构建官网，最后执行 `wrangler deploy` 上传静态资源并发布。Wrangler 会显示实际访问 URL。常规 `build` 不同步快照，仍可离线构建。需要仅验证产物和配置时，可运行：

```sh
make website:build
pnpm --dir website exec wrangler deploy --dry-run
```

在 CI 或其他非交互环境中，将 `CLOUDFLARE_API_TOKEN` 和 `CLOUDFLARE_ACCOUNT_ID` 配置为受保护的环境变量，再执行相同部署命令。Token 需有目标账户的 Workers 部署权限；不要将 Token 写入仓库、前端代码或构建参数。若需要自定义域名，可在 Cloudflare 控制台为此 Worker 绑定域名，或在 `wrangler.jsonc` 配置对应的 `routes`。

`public/_headers` 配置基础响应头及 Vite 内容哈希资源的长期缓存，Vite 构建时会将其复制到 `dist/`。`robots.txt` 不包含尚未确定的域名。

Cloudflare 文档：[静态资源配置](https://developers.cloudflare.com/workers/static-assets/binding/)、[自定义响应头](https://developers.cloudflare.com/workers/static-assets/headers/)、[Wrangler 登录](https://developers.cloudflare.com/workers/wrangler/commands/general/#login)、[CI/CD](https://developers.cloudflare.com/workers/ci-cd/external-cicd/)。
