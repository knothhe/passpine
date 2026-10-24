import type { PrivacySectionId } from "./routes.ts";

export const homebrewInstallCommand = "brew install --cask knothhe/tap/passpine";

// Policy sections are structured so both languages keep the same shape.
export type PolicyListItem = string | { term: string; text: string };
export type PolicyBlock =
  | { kind: "paragraph"; text: string }
  | { kind: "list"; items: PolicyListItem[] }
  | {
      kind: "table";
      head: [string, string];
      rows: { name: string; text: string }[];
    };
export type PolicySection = { title: string; blocks: PolicyBlock[] };

const paragraph = (text: string): PolicyBlock => ({ kind: "paragraph", text });
const list = (...items: PolicyListItem[]): PolicyBlock => ({
  kind: "list",
  items,
});
const table = (
  head: [string, string],
  rows: { name: string; text: string }[],
): PolicyBlock => ({ kind: "table", head, rows });

export const zh = {
  meta: {
    home: {
      title: "Passpine — 兼容 pass 的本地密码管理器",
      description:
        "兼容 pass 的本地密码管理器。原生桌面客户端、密码生成、TOTP、Git 同步与浏览器填充。下载 macOS、Windows 和 Linux 版本。",
    },
    privacy: {
      title: "隐私政策 — Passpine",
      description:
        "Passpine 桌面客户端、浏览器扩展与官网如何处理信息：数据保存在本机，没有账户、广告、分析或遥测。",
    },
    notFound: {
      title: "页面不存在 — Passpine",
      description: "你访问的 Passpine 页面不存在。",
    },
  },
  brand: "Passpine",
  skip: "跳到主要内容",
  nav: {
    features: "功能",
    security: "隐私与安全",
    guide: "开始使用",
    faq: "常见问题",
    download: "下载",
    source: "GitHub",
    home: "Passpine 首页",
    label: "主导航",
  },
  language: {
    label: "网站语言",
    system: "跟随系统",
    en: "English",
    zh: "简体中文",
  },
  hero: {
    eyebrow: "本地优先 · 兼容 pass",
    title: "Passpine",
    description:
      "兼容 pass 的本地密码管理器，支持密码生成、TOTP、Git 同步与浏览器填充。",
    download: "下载 Passpine",
    downloadFor: (platform: string, architecture: string) =>
      `下载 ${platform} · ${architecture}`,
    explore: "了解它的不同",
    platforms: "macOS · Windows · Linux",
    preview: "真实客户端与扩展预览 · 仅使用演示账号",
    desktopAlt: "Passpine 桌面客户端：文件夹、账号列表与 GitHub 演示账号详情",
    extensionAlt: "Passpine 浏览器扩展：搜索和填充演示账号",
    points: ["无需注册账户", "密码条目本地加密", "沿用标准 pass 格式"],
  },
  video: {
    eyebrow: "VIDEO",
    title: "30 秒看 Passpine 怎么用",
    description: "填充登录、生成密码、Git 同步，以及导入已有的 pass 密码库。",
    label: "Passpine 产品演示视频",
    unsupported: "当前浏览器无法播放此视频。",
  },
  features: {
    eyebrow: "FEATURES",
    title: "日常需要的功能",
    description:
      "密码、验证码与日常登录，各得其所。熟悉的桌面体验，保留属于你的控制权。",
    items: [
      {
        title: "先在本地，安心使用",
        body: "密码条目以 OpenPGP 加密保存在自己的设备上。没有网络，也能打开你的密码库。",
        tag: "LOCAL FIRST",
      },
      {
        title: "好密码，不必费心记",
        body: "生成强密码，将 TOTP 验证码和自定义字段放在同一个条目里。需要的时候，一处找到。",
        tag: "EVERYDAY ESSENTIALS",
      },
      {
        title: "从密码库，到登录框",
        body: "通过扩展搜索并填充账号。支持 Chrome、Edge、Brave 和 Firefox，客户端需保持运行。",
        tag: "BROWSER COMPANION",
      },
      {
        title: "同步，按你的方式",
        body: "按需开启 Git 同步，选择自己的远端仓库。私钥留在本机，不进入同步仓库。",
        tag: "YOUR GIT, YOUR CHOICE",
      },
      {
        title: "熟悉的 pass，自然衔接",
        body: "沿用 .gpg 与 .gpg-id 格式。打开已有密码库，也可以从零创建，无需事先安装 GnuPG。",
        tag: "BUILT ON OPEN FORMATS",
      },
      {
        title: "随时唤起，随手找到",
        body: "通过全局快捷搜索快速找到条目。文件夹、草稿与回收站，让日常整理井井有条。",
        tag: "NATIVE DESKTOP",
      },
    ],
  },
  security: {
    eyebrow: "PRIVACY & SECURITY",
    title: "本地存储，密钥自管",
    description:
      "密码库是你能看见的文件。密钥由你保管，同步由你决定。无需先把密码交给一个托管账户。",
    link: "阅读架构与安全设计",
    privacyLink: "阅读隐私政策",
    points: [
      {
        title: "OpenPGP 加密",
        body: "新建库自动生成受口令保护的私钥；密码条目加密后写入磁盘。",
      },
      {
        title: "恢复权，在你手中",
        body: "备份密码库、恢复私钥和口令。丢失密钥或口令后，没有服务器能替你找回。",
      },
      {
        title: "边界，清楚可见",
        body: "条目内容加密，文件名、目录结构和 Git 元数据仍可见。请据此选择条目名称和同步位置。",
      },
    ],
    treeTitle: "你的密码库",
    treeBadge: "条目内容已加密",
    treeRoot: "~/.password-store/",
    treeFolder: "工作/",
    treeFiles: ["GitHub.gpg", "Cloudflare.gpg"],
    treeRecipient: ".gpg-id",
    keyTitle: "私钥单独保存在应用数据目录",
    keyBody: "不进入 Git 同步仓库",
    footnote: "示意目录 · 文件名不加密",
  },
  guide: {
    eyebrow: "GET STARTED",
    title: "三步开始使用",
    items: [
      {
        title: "安顿好你的密码库",
        body: "创建新库、打开本地 pass 密码库，或从 Git 仓库下载。新建库时设置至少 12 个字符的口令。",
      },
      {
        title: "给自己留一份备份",
        body: "导出并验证恢复私钥，妥善保存密码库、私钥和口令，再开始迁入日常账号。",
      },
      {
        title: "让登录更顺手",
        body: "安装浏览器扩展，在客户端设置中修复浏览器连接。按需启用 Git 同步。",
      },
    ],
    link: "查看完整安装指南",
  },
  downloads: {
    eyebrow: "DOWNLOAD",
    title: "下载 Passpine",
    description: "选择适合你桌面的版本。安装包直接来自 GitHub Releases。",
    platforms: { macos: "macOS", windows: "Windows", linux: "Linux" },
    architectures: { arm64: "ARM64", x64: "x64" },
    macArchitectures: { arm64: "Apple Silicon", x64: "Intel" },
    detected: (platform: string) =>
      `已识别 ${platform}。以下仅展示已发布的安装包，请留意处理器架构。`,
    currentSystem: "当前系统",
    unknown: "以下为正式版实际提供的安装包，请选择适合系统与处理器的版本。",
    loading: "正在获取最新正式版…",
    error: "暂时无法获取最新版本，以下下载来自已保存的正式版。",
    retry: "重新获取",
    latest: (tag: string) => `${tag} · 最新正式版`,
    savedVersion: (tag: string) => `${tag} · 可用正式版`,
    releases: "在 GitHub 查看全部版本",
    download: (platform: string, architecture: string) =>
      `下载 ${platform} · ${architecture}`,
    unavailable: "此版本未提供桌面安装包，请查看 GitHub 发布说明。",
    mobile: "这是桌面应用，请在电脑上安装。你仍可查看各系统的下载选项。",
    notes: {
      macos: "macOS 应用尚未进行 Developer ID 公证。请留意安装包的处理器架构。",
      windows:
        "Windows 安装包尚未进行 Authenticode 签名，系统可能显示安全提示。",
      linux:
        "Linux 需要 GTK 3 与 WebKitGTK 4.1。下载压缩包后，按安装指南运行安装脚本。",
    },
    installGuide: "安装与系统要求",
    homebrew: "通过 Homebrew 安装",
    homebrewNote:
      "适用于 Apple Silicon · macOS 13 及以上。命令会自动添加 knothhe/tap 并安装正式版。",
    extensionTitle: "把 Passpine 带进浏览器",
    extensionBody:
      "先安装并运行桌面客户端，再按指南加载扩展。当前提供本地安装包。",
    chromium: "Chrome / Edge / Brave",
    firefox: "Firefox",
    extensionDownload: (name: string) => `下载 ${name} 扩展`,
    extensionGuide: "扩展安装指南",
  },
  faq: {
    title: "你可能还想知道",
    items: [
      {
        question: "我没用过 pass，也能使用吗？",
        answer:
          "可以。创建新库时，Passpine 会自动生成受口令保护的 OpenPGP 私钥，无需安装 GnuPG，也不需要使用命令行。请记住口令并验证恢复私钥备份。",
      },
      {
        question: "已有的 pass 密码库怎么迁入？",
        answer:
          "直接打开本地密码库，或从 Git 远端下载。可通过系统 GPG 按 .gpg-id 匹配密钥，也可导入私钥文件；多收件人库需要相应公钥。暂不支持硬件卡私钥。",
      },
      {
        question: "Git 同步是必须的吗？",
        answer:
          "不是。你可以只在本机使用。需要多设备同步时，再安装 Git，配置自己的远端仓库和系统认证凭据。同步内容是加密条目，私钥需单独备份与迁移。",
      },
      {
        question: "浏览器扩展可以独立使用吗？",
        answer:
          "扩展需要已安装且正在运行的桌面客户端，通过 Native Messaging 连接本地密码库。当前从 GitHub 下载后本地加载；Firefox 临时扩展在浏览器重启后需要重新加载。填写账号后不会自动提交网页表单。",
      },
      {
        question: "忘记口令或丢失密钥怎么办？",
        answer:
          "Passpine 没有托管账户或服务器找回功能。请在开始使用前，导出并验证恢复私钥，同时备份密码库和口令。只有加密密码库而没有可用密钥与口令，无法恢复内容。",
      },
    ],
  },
  footer: {
    tagline: "留一点简单，给数字生活。",
    source: "查看源码",
    releases: "更新记录",
    issues: "反馈问题",
    docs: "项目文档",
    privacy: "隐私政策",
    label: "页脚链接",
    note: "本地优先。由你掌握。",
  },
  privacy: {
    eyebrow: "PRIVACY POLICY",
    title: "隐私政策",
    intro:
      "Passpine 没有账户系统，也不运营接收你密码库或浏览记录的服务器。本政策说明桌面客户端、浏览器扩展和本网站会在何时、为何处理哪些信息。",
    updated: (date: string) => `最后更新 ${date}`,
    reviewed: (version: string) => `依据 v${version} 核对`,
    source: "在 GitHub 查看源码",
    summaryTitle: "要点",
    summary: [
      {
        title: "没有账户，没有开发者服务器",
        body: "无需注册。开发者无法访问你的密码库、浏览记录或使用情况。",
      },
      {
        title: "没有广告、分析或遥测",
        body: "客户端、扩展和本网站都不包含广告、分析 SDK 或使用数据上传。",
      },
      {
        title: "数据留在你的设备",
        body: "条目以 OpenPGP 加密保存在你选择的目录；扩展只与同一台电脑上的客户端通信。",
      },
      {
        title: "联网，清楚可见",
        body: "填充和 Git 同步由你触发；正式版客户端每天向 GitHub 检查一次更新。",
      },
    ],
    tocLabel: "目录",
    sections: {
      scope: {
        title: "适用范围",
        blocks: [
          paragraph(
            "本政策适用于 Passpine 浏览器扩展（Chrome、Edge、Brave 与 Firefox）、配套的桌面客户端以及本网站。Passpine 由 knothhe 开发和维护，源码公开在 GitHub，你可以据此核对这里描述的行为。",
          ),
          paragraph(
            "扩展需要已安装且正在运行的桌面客户端。密码库由客户端管理；扩展让你搜索和编辑账号、复制凭证、生成密码，并在你操作时填写当前网页的登录字段。",
          ),
        ],
      },
      data: {
        title: "处理哪些信息",
        blocks: [
          paragraph("为提供上述功能，Passpine 在你的设备上处理以下信息："),
          list(
            {
              term: "密码库信息",
              text: "库名称、条目名称、目录和锁定状态，用于显示和查找账号。客户端默认允许锁定时浏览条目名称，可在客户端设置中关闭。",
            },
            {
              term: "条目内容",
              text: "你保存的用户名、密码、网站地址、一次性验证码、备注和自定义字段。复制和网页填充只使用用户名、密码或验证码。",
            },
            {
              term: "当前网页",
              text: "你打开扩展时当前标签页的网址和主机名，用于匹配账号，并在填充前确认页面没有变化。你要求填充时，扩展检查登录输入框的类型、属性、可见性和表单结构，不读取整页正文。",
            },
            {
              term: "偏好设置",
              text: "语言、外观、字号、快捷键，以及账号与网站的关联决定。",
            },
            {
              term: "本机操作记录",
              text: "客户端记录操作的时间、名称、来源和结果，方便你查看近期操作；记录中不包含密码、条目正文或网页内容。",
            },
            {
              term: "密钥",
              text: "你在客户端创建或导入的密钥身份信息、私钥和口令，用于加密和解锁密码库。扩展不持有私钥，也不建立独立的解锁会话。",
            },
          ),
          paragraph(
            "客户端和扩展不包含广告、分析 SDK 或遥测，也没有向开发者服务器上传密码库、浏览记录或使用情况的接口。扩展不会持续记录你访问的网页。",
          ),
        ],
      },
      storage: {
        title: "保存与保护",
        blocks: [
          list(
            "条目内容以 OpenPGP 加密的 .gpg 文件保存在你选择的密码库目录。条目路径、文件名、.gpg-id 收件人标识和 Git 元数据不会因此被隐藏。",
            "私钥以受口令保护的形式保存在客户端应用数据目录，与同步的密码库分开。系统解锁如已启用，由客户端和操作系统处理。",
            "扩展通过 Native Messaging 与本机的 Native Host 通信，Native Host 再通过带令牌验证的本机回环连接转发给客户端；这段本机连接不使用 TLS。",
            "展示或填充所需的解密内容只短暂存在于扩展内存中。弹窗隐藏、锁定、断开连接或切换密码库时，扩展会清理相关界面状态；这不保证操作系统或运行时中的每份内存副本都被立即擦除。",
            "偏好和网站关联保存在浏览器本地存储中，不使用浏览器同步存储，也不保存密码或解密后的条目内容。网站关联以密码库 ID、条目路径和精确主机名计算出的 SHA-256 键加一个是否关联的值保存；哈希不等于加密或匿名化。完整网址不会作为浏览记录保存。偏好和关联没有自动过期时间。",
            "操作记录只保存在本机，默认开启，最多保留 7 天、1,000 条，可调整或关闭；过期记录在客户端运行期间定期清理。",
          ),
        ],
      },
      sharing: {
        title: "信息何时离开你的设备",
        blocks: [
          paragraph(
            "Passpine 不出售、出租或交换你的信息。只有以下情况，信息会离开 Passpine：",
          ),
          list(
            {
              term: "网页填充",
              text: "你要求填充时，所选用户名、密码或验证码会写入当前网页，网页脚本可以读取并按网站自身逻辑处理。Passpine 不会替你提交表单。HTTP 网页缺少 HTTPS 的传输保护，填充前请核对网站。",
            },
            {
              term: "剪贴板",
              text: "复制的内容会进入系统剪贴板，其他有权限的软件或剪贴板历史可能读取。客户端默认在 30 秒后，且剪贴板仍是该值时将其清空；已被其他软件保存的副本无法清除。",
            },
            {
              term: "Git 同步",
              text: "默认关闭。你配置远端并手动同步或开启自动同步后，客户端使用系统 Git 和你已有的认证配置连接你指定的仓库，传输加密的库文件及可见的路径、收件人标识和 Git 元数据。托管方按其自身政策处理账户、访问日志与数据保留。私钥不进入同步仓库。",
            },
            {
              term: "更新检查",
              text: "正式版客户端每天从 GitHub Releases 获取一次已签名的更新清单，默认自动下载更新包，可在设置中关闭自动下载。请求不含密码库内容、账户或设备标识；与任何下载一样，GitHub 能看到你的 IP 地址和请求信息。",
            },
            {
              term: "你打开的链接",
              text: "打开项目链接、GitHub Release 页面或提交反馈时，目标服务按其自身政策处理访问和你提供的信息。",
            },
          ),
        ],
      },
      permissions: {
        title: "扩展权限",
        blocks: [
          paragraph(
            "扩展基础功能使用以下权限。网页快捷键为可选功能：你在设置页启用并授权后，扩展才在 HTTP / HTTPS 网页运行快捷键监听脚本；可随时关闭并撤销网站访问。",
          ),
          table(
            ["权限", "用途"],
            [
              {
                name: "nativeMessaging",
                text: "与单独安装的 Passpine 桌面客户端通信。客户端负责密码库、解锁会话、加解密、密码生成和凭证复制。",
              },
              {
                name: "activeTab",
                text: "在你打开扩展后访问当前标签页，用于显示主机名、匹配账号并确认填充目标没有变化，无需长期访问所有网站。",
              },
              {
                name: "scripting",
                text: "在你要求填充时向当前页面顶层注入自带的填充函数，不提交表单；网页快捷键获授权后，用于注册和运行快捷键监听脚本。",
              },
              {
                name: "storage",
                text: "在浏览器本地保存语言、外观、快捷键和网站关联决定。",
              },
              {
                name: "optional_host_permissions",
                text: "可选的 http://*/* 与 https://*/* 网站访问，用于在普通网页监听已配置的快捷键，以及在你主动触发时匹配、填充账号。快捷键监听器跳过文字编辑，不记录或上传输入内容，也不接收密码库条目或解密凭证。填充的值仍可被当前网页读取。",
              },
              {
                name: "clipboardWrite",
                text: "在连接设置中复制扩展 ID，便于配置和排查。凭证复制由客户端处理；扩展不读取剪贴板。",
              },
            ],
          ),
          paragraph(
            "扩展的全部可执行代码都随安装包分发，不下载或执行远程代码。",
          ),
        ],
      },
      website: {
        title: "本网站",
        blocks: [
          list(
            "本网站不使用 Cookie、广告或第三方分析。你选择的网站语言保存在浏览器本地存储中。",
            "下载区由你的浏览器直接向 GitHub API 获取最新版本信息；下载安装包时会连接 GitHub。",
            "宣传视频的封面随页面加载，视频只在你播放时加载，两者均来自 oss.knothhe.com。",
            "网站与视频由 Cloudflare 托管。Cloudflare 可能为安全和运营目的处理请求日志，例如 IP 地址和浏览器信息。",
          ),
        ],
      },
      limits: {
        title: "使用限制",
        blocks: [
          paragraph(
            "Passpine 只为上述密码管理功能使用用户数据：不出售用户数据，不用于广告或个性化推荐，不用于评估信用或放贷，也不转交给数据经纪人。",
          ),
          paragraph(
            "Passpine 对从 Google API 获取的信息的使用，遵守 Chrome Web Store 用户数据政策，包括“有限使用”要求。",
          ),
        ],
      },
      choices: {
        title: "你的选择与删除",
        blocks: [
          list(
            "你可以随时修改条目、取消网站关联、调整偏好、锁定会话、关闭操作记录或卸载扩展。",
            "删除的账号会先移入客户端的加密回收站，可以恢复；在客户端永久删除后才会移除该副本。",
            "卸载扩展会移除它在浏览器中的本地数据，但不会删除密码库、客户端数据、导出的恢复私钥、系统备份、剪贴板历史或 Git 远端。这些数据需要在客户端、文件系统和你选择的服务中分别处理；删除本地工作副本不会删除 Git 历史或远端备份。",
            "由于没有托管账户，开发者无法访问、导出或删除你设备上的数据，也无法替你找回口令或私钥。",
          ),
        ],
      },
      changes: {
        title: "政策更新",
        blocks: [
          paragraph(
            "当 Passpine 处理信息的方式发生变化时，我们会更新本政策和页首日期。本政策随网站源码保存在 GitHub，历史版本可在提交记录中查看。",
          ),
        ],
      },
      contact: {
        title: "联系我们",
        blocks: [
          paragraph(
            "对本政策或 Passpine 的一般问题，可以在 GitHub Issues 中公开提出。涉及个人信息、隐私请求或安全漏洞的问题，请发送邮件私下联系维护者。",
          ),
          paragraph(
            "无论通过哪种方式，都请不要发送密码、私钥、口令、验证码、密码库文件或其他敏感信息。",
          ),
        ],
      },
    } satisfies Record<PrivacySectionId, PolicySection>,
    contactLink: "前往 GitHub Issues",
    contactEmail: (email: string) => `发送邮件至 ${email}`,
    backToTop: "回到顶部",
  },
  notFound: {
    eyebrow: "404",
    title: "这个页面不存在",
    description: "链接可能已失效，或页面已经移动。",
    home: "返回首页",
    privacy: "查看隐私政策",
  },
};

export type Copy = typeof zh;

export const en: Copy = {
  meta: {
    home: {
      title: "Passpine — A local password manager compatible with pass",
      description:
        "A local password manager compatible with pass. Native desktop apps, password generation, TOTP, Git sync, and browser filling. Download for macOS, Windows, and Linux.",
    },
    privacy: {
      title: "Privacy Policy — Passpine",
      description:
        "How the Passpine desktop app, browser extension, and website handle information: data stays on your device, with no accounts, ads, analytics, or telemetry.",
    },
    notFound: {
      title: "Page not found — Passpine",
      description: "The Passpine page you’re looking for doesn’t exist.",
    },
  },
  brand: "Passpine",
  skip: "Skip to main content",
  nav: {
    features: "Features",
    security: "Privacy",
    guide: "Get started",
    faq: "FAQ",
    download: "Download",
    source: "GitHub",
    home: "Passpine home",
    label: "Main navigation",
  },
  language: {
    label: "Website language",
    system: "System",
    en: "English",
    zh: "简体中文",
  },
  hero: {
    eyebrow: "LOCAL FIRST · PASS COMPATIBLE",
    title: "Passpine",
    description:
      "A local password manager compatible with pass, with password generation, TOTP, Git sync, and browser filling.",
    download: "Download Passpine",
    downloadFor: (platform, architecture) =>
      `Download for ${platform} · ${architecture}`,
    explore: "See what makes it different",
    platforms: "macOS · Windows · Linux",
    preview: "Real desktop and extension previews · Demo accounts only",
    desktopAlt:
      "Passpine desktop showing folders, accounts, and a demo GitHub entry",
    extensionAlt: "Passpine browser extension showing search and demo accounts",
    points: [
      "No account to create",
      "Entries encrypted locally",
      "Standard pass format",
    ],
  },
  video: {
    eyebrow: "VIDEO",
    title: "Passpine in 30 seconds",
    description:
      "Fill sign-ins, generate passwords, sync with Git, and import an existing pass vault.",
    label: "Passpine product demo video",
    unsupported: "This browser can't play the video.",
  },
  features: {
    eyebrow: "FEATURES",
    title: "The everyday essentials",
    description:
      "Passwords, verification codes, and everyday sign-ins, all in their place. A familiar desktop experience that keeps you in control.",
    items: [
      {
        title: "At home on your device",
        body: "Password entries are encrypted with OpenPGP and stored on your device. Open your vault even when you’re offline.",
        tag: "LOCAL FIRST",
      },
      {
        title: "Less to remember",
        body: "Generate strong passwords and keep TOTP codes and custom fields alongside your accounts. Find what you need in one place.",
        tag: "EVERYDAY ESSENTIALS",
      },
      {
        title: "From vault to sign-in",
        body: "Search and fill accounts with the browser extension. Works with Chrome, Edge, Brave, and Firefox while the desktop app is running.",
        tag: "BROWSER COMPANION",
      },
      {
        title: "Sync on your terms",
        body: "Enable Git sync when you need it, using a remote you choose. Your private key stays out of the sync repository.",
        tag: "YOUR GIT, YOUR CHOICE",
      },
      {
        title: "An open, familiar format",
        body: "Keep the .gpg and .gpg-id format. Open an existing pass vault or create a new one without installing GnuPG first.",
        tag: "BUILT ON OPEN FORMATS",
      },
      {
        title: "Just a shortcut away",
        body: "Find an entry with global quick search. Keep things organized with folders, encrypted drafts, and trash.",
        tag: "NATIVE DESKTOP",
      },
    ],
  },
  security: {
    eyebrow: "PRIVACY & SECURITY",
    title: "Local files. Your own keys.",
    description:
      "Your vault is made of files you can see. You hold the keys and choose how to sync. No hosted account to hand your passwords to first.",
    link: "Read the architecture and security design",
    privacyLink: "Read the privacy policy",
    points: [
      {
        title: "OpenPGP encryption",
        body: "New vaults generate a passphrase-protected private key. Password entries are encrypted before being written to disk.",
      },
      {
        title: "Recovery stays with you",
        body: "Back up your vault, recovery key, and passphrase. No server can recover a missing key or forgotten passphrase for you.",
      },
      {
        title: "Clear about the boundaries",
        body: "Entry contents are encrypted; filenames, folders, and Git metadata remain visible. Choose entry names and sync locations accordingly.",
      },
    ],
    treeTitle: "Your password store",
    treeBadge: "Entry contents encrypted",
    treeRoot: "~/.password-store/",
    treeFolder: "work/",
    treeFiles: ["GitHub.gpg", "Cloudflare.gpg"],
    treeRecipient: ".gpg-id",
    keyTitle: "Private key in the app data directory",
    keyBody: "Outside the Git sync repository",
    footnote: "Illustrative directory · Filenames are not encrypted",
  },
  guide: {
    eyebrow: "GET STARTED",
    title: "Get started in three steps",
    items: [
      {
        title: "Make a home for your passwords",
        body: "Create a vault, open a local pass store, or clone one from Git. Choose a passphrase of at least 12 characters for a new vault.",
      },
      {
        title: "Give yourself a way back",
        body: "Export and verify your recovery key. Safely back up your vault, key, and passphrase before moving in your everyday accounts.",
      },
      {
        title: "Make sign-ins feel effortless",
        body: "Install the browser extension and repair its connection in desktop settings. Enable Git sync whenever you need it.",
      },
    ],
    link: "Read the complete installation guide",
  },
  downloads: {
    eyebrow: "DOWNLOAD",
    title: "Download Passpine",
    description:
      "Find the right version for your desktop. Downloads come directly from GitHub Releases.",
    platforms: { macos: "macOS", windows: "Windows", linux: "Linux" },
    architectures: {
      arm64: "ARM64",
      x64: "x64",
    },
    macArchitectures: { arm64: "Apple Silicon", x64: "Intel" },
    detected: (platform) =>
      `${platform} detected. Only published installers are listed below; check the processor architecture.`,
    currentSystem: "Your system",
    unknown:
      "Only published installers are listed. Choose the one matching your system and processor.",
    loading: "Fetching the latest stable release…",
    error:
      "We can’t check the latest version right now. Downloads below are from the saved stable release.",
    retry: "Try again",
    latest: (tag) => `${tag} · Latest stable release`,
    savedVersion: (tag) => `${tag} · Available stable release`,
    releases: "View all releases on GitHub",
    download: (platform, architecture) =>
      `Download for ${platform} · ${architecture}`,
    unavailable:
      "This release has no desktop installers. See the release notes on GitHub.",
    mobile:
      "Passpine is a desktop app. Install it on a computer; you can still explore the downloads here.",
    notes: {
      macos:
        "The macOS app is not Developer ID notarized. Check the installer’s processor architecture.",
      windows:
        "The Windows installer is not Authenticode signed. Your system may display a security notice.",
      linux:
        "Linux requires GTK 3 and WebKitGTK 4.1. Extract the archive and follow the guide to run the installer.",
    },
    installGuide: "Installation and system requirements",
    homebrew: "Install with Homebrew",
    homebrewNote:
      "For Apple Silicon · macOS 13 or later. The command adds knothhe/tap and installs the stable app.",
    extensionTitle: "A companion for your browser",
    extensionBody:
      "Install and run the desktop app first, then load the extension using the guide. Local installation packages are available now.",
    chromium: "Chrome / Edge / Brave",
    firefox: "Firefox",
    extensionDownload: (name) => `Download ${name} extension`,
    extensionGuide: "Extension installation guide",
  },
  faq: {
    title: "A few things you might be wondering",
    items: [
      {
        question: "Do I need to know how to use pass?",
        answer:
          "No. Passpine generates a passphrase-protected OpenPGP private key when you create a vault. No GnuPG installation or command line needed. Keep your passphrase safe and verify your recovery key backup.",
      },
      {
        question: "Can I use an existing pass vault?",
        answer:
          "Open a local vault or clone it from Git. Match keys to .gpg-id using system GPG, or import a private key file. Multiple-recipient vaults need the corresponding public keys. Hardware-card private keys are not supported yet.",
      },
      {
        question: "Is Git sync required?",
        answer:
          "No. You can keep everything local. For sync across devices, install Git and configure your own remote and system authentication. Sync transfers encrypted entries; back up and transfer private keys separately.",
      },
      {
        question: "Does the browser extension work on its own?",
        answer:
          "The desktop app needs to be installed and running. The extension connects to your local vault through Native Messaging. Download and load the extension locally; Firefox temporary extensions need to be loaded again after a browser restart. Filling an account does not submit the form.",
      },
      {
        question: "What if I lose my key or forget my passphrase?",
        answer:
          "Passpine has no hosted account or server recovery. Export and verify your recovery key before getting started, and back up the vault and passphrase. An encrypted vault cannot be recovered without a usable key and passphrase.",
      },
    ],
  },
  footer: {
    tagline: "A little more calm for your digital life.",
    source: "Source code",
    releases: "Changelog",
    issues: "Report an issue",
    docs: "Documentation",
    privacy: "Privacy policy",
    label: "Footer links",
    note: "Local first. Yours to keep.",
  },
  privacy: {
    eyebrow: "PRIVACY POLICY",
    title: "Privacy Policy",
    intro:
      "Passpine has no account system and runs no server that receives your vault or browsing activity. This policy explains what the desktop app, the browser extension, and this website handle, and when and why.",
    updated: (date) => `Last updated ${date}`,
    reviewed: (version) => `Checked against v${version}`,
    source: "View the source on GitHub",
    summaryTitle: "At a glance",
    summary: [
      {
        title: "No accounts, no developer servers",
        body: "Nothing to sign up for. The developer cannot see your vault, browsing, or usage.",
      },
      {
        title: "No ads, analytics, or telemetry",
        body: "The desktop app, extension, and website contain no ads, analytics SDKs, or usage uploads.",
      },
      {
        title: "Your data stays on your device",
        body: "Entries are OpenPGP-encrypted in a folder you choose. The extension talks only to the app on the same computer.",
      },
      {
        title: "Network use you can see",
        body: "You trigger filling and Git sync. Release builds of the desktop app check GitHub for updates once a day.",
      },
    ],
    tocLabel: "On this page",
    sections: {
      scope: {
        title: "Scope",
        blocks: [
          paragraph(
            "This policy covers the Passpine browser extension (Chrome, Edge, Brave, and Firefox), its companion desktop app, and this website. Passpine is developed and maintained by knothhe. Its source code is public on GitHub, so you can verify the behavior described here.",
          ),
          paragraph(
            "The extension requires the desktop app to be installed and running. The app manages your vault; the extension lets you search and edit accounts, copy credentials, generate passwords, and fill login fields on the current page when you ask it to.",
          ),
        ],
      },
      data: {
        title: "Information we handle",
        blocks: [
          paragraph(
            "To provide these features, Passpine handles the following on your device:",
          ),
          list(
            {
              term: "Vault details",
              text: "Vault and entry names, folders, and lock status, to list and find accounts. By default the app lets you browse entry names while locked; you can turn this off in its settings.",
            },
            {
              term: "Entry contents",
              text: "Usernames, passwords, website addresses, one-time codes, notes, and custom fields you save. Copying and filling use only the username, password, or one-time code.",
            },
            {
              term: "The current page",
              text: "The active tab’s URL and hostname when you open the extension, to match accounts and to confirm the page hasn’t changed before filling. When you ask to fill, the extension inspects login inputs’ types, attributes, visibility, and form structure. It doesn’t read the page’s content.",
            },
            {
              term: "Preferences",
              text: "Language, appearance, text size, shortcuts, and your account-to-website associations.",
            },
            {
              term: "Local activity records",
              text: "The app records each action’s time, name, source, and result so you can review recent activity. Records never contain passwords, entry contents, or page contents.",
            },
            {
              term: "Keys",
              text: "Key identity information, private keys, and passphrases you create or import in the app, used to encrypt and unlock your vault. The extension holds no private keys and has no separate unlock session.",
            },
          ),
          paragraph(
            "The app and extension contain no advertising, analytics SDKs, or telemetry, and have no interface that uploads your vault, browsing, or usage to a developer server. The extension doesn’t continuously record the pages you visit.",
          ),
        ],
      },
      storage: {
        title: "Storage and protection",
        blocks: [
          list(
            "Entry contents are stored as OpenPGP-encrypted .gpg files in the vault folder you choose. Entry paths, filenames, .gpg-id recipient identifiers, and Git metadata are not hidden by this encryption.",
            "Private keys are passphrase-protected and kept in the app’s data directory, separate from the synchronized vault. If enabled, system unlock is handled by the app and the operating system.",
            "The extension uses Native Messaging to reach the local Native Host, which relays requests to the app over a token-authenticated loopback connection. This local connection doesn’t use TLS.",
            "Decrypted content needed for display or filling exists only briefly in extension memory. Related UI state is cleared when the popup closes, the vault locks or disconnects, or you switch vaults. This doesn’t guarantee immediate erasure of every copy in runtime or operating-system memory.",
            "Preferences and associations use local browser storage, never browser sync storage, and never store passwords or decrypted entries. An association is saved as a SHA-256 key derived from the vault ID, entry path, and exact hostname, plus a yes/no value; hashing is not encryption or anonymization. Full URLs are not saved as browsing history. Preferences and associations don’t expire automatically.",
            "Activity records stay on your device. Recording is on by default and keeps up to 7 days or 1,000 records; you can adjust or disable it. Expired records are pruned while the app is running.",
          ),
        ],
      },
      sharing: {
        title: "When information leaves your device",
        blocks: [
          paragraph(
            "Passpine doesn’t sell, rent, or trade your information. Information leaves Passpine only in these cases:",
          ),
          list(
            {
              term: "Filling",
              text: "When you ask to fill, the selected username, password, or code is written into the current page, where its scripts can read and process it under the site’s own logic. Passpine never submits the form for you. HTTP pages lack HTTPS transport protection, so check the site before filling.",
            },
            {
              term: "Clipboard",
              text: "Copied values go to the system clipboard, where other permitted software or clipboard history may read them. By default, the app clears the value after 30 seconds if it hasn’t been replaced. Copies already kept by other software can’t be removed.",
            },
            {
              term: "Git sync",
              text: "Off by default. Once you configure a remote and sync manually or turn on automatic sync, the app uses system Git and your existing authentication to reach the repository you chose, transferring encrypted vault files along with visible paths, recipient identifiers, and Git metadata. Your hosting provider handles accounts, access logs, and retention under its own policy. Private keys stay out of the synchronized repository.",
            },
            {
              term: "Update checks",
              text: "Release builds of the desktop app fetch a signed update manifest from GitHub Releases once a day and, by default, download updates automatically; you can turn off automatic downloads in settings. Requests contain no vault contents, account, or device identifiers. As with any download, GitHub can see your IP address and request details.",
            },
            {
              term: "Links you open",
              text: "When you open project links or GitHub Release pages, or send feedback, the destination handles your visit and what you provide under its own policy.",
            },
          ),
        ],
      },
      permissions: {
        title: "Extension permissions",
        blocks: [
          paragraph(
            "The core extension uses the permissions below. Page shortcuts are optional: only after you enable them and grant website access does a shortcut listener run on HTTP / HTTPS pages. You can turn them off and revoke access at any time.",
          ),
          table(
            ["Permission", "Purpose"],
            [
              {
                name: "nativeMessaging",
                text: "Communicate with the separately installed Passpine desktop app, which owns the vault and unlock session, encryption, password generation, and credential copying.",
              },
              {
                name: "activeTab",
                text: "Access the current tab after you open the extension, to show its hostname, match accounts, and confirm the fill target hasn’t changed, without persistent access to every website.",
              },
              {
                name: "scripting",
                text: "Inject the packaged fill function into the page’s top frame when you request filling, without submitting forms. After you grant page shortcuts access, also register and run the shortcut listener.",
              },
              {
                name: "storage",
                text: "Save language, appearance, shortcuts, and website associations in local browser storage.",
              },
              {
                name: "optional_host_permissions",
                text: "Optional http://*/* and https://*/* website access to listen for configured shortcuts and match or fill an account when you trigger them. The shortcut listener skips text editing, does not record or upload typed content, and receives no vault entries or decrypted credentials. Filled values can still be read by the current page.",
              },
              {
                name: "clipboardWrite",
                text: "Copy the extension ID from connection settings for setup and troubleshooting. Credentials are copied by the desktop app; the extension never reads the clipboard.",
              },
            ],
          ),
          paragraph(
            "All executable code ships inside the extension package. It doesn’t download or run remote code.",
          ),
        ],
      },
      website: {
        title: "This website",
        blocks: [
          list(
            "This website uses no cookies, ads, or third-party analytics. Your language choice is saved in local browser storage.",
            "The download section has your browser fetch the latest release details directly from the GitHub API, and installers download from GitHub.",
            "The promo video’s poster loads with the page and the video loads only when you play it, both from oss.knothhe.com.",
            "The website and video are hosted on Cloudflare, which may process request logs, such as IP addresses and browser details, for security and operations.",
          ),
        ],
      },
      limits: {
        title: "Limited use",
        blocks: [
          paragraph(
            "Passpine uses user data only for the password-management features described above. It doesn’t sell user data, use it for advertising or personalization, use it to determine creditworthiness or for lending, or transfer it to data brokers.",
          ),
          paragraph(
            "The use of information received from Google APIs will adhere to the Chrome Web Store User Data Policy, including the Limited Use requirements.",
          ),
        ],
      },
      choices: {
        title: "Your choices and deletion",
        blocks: [
          list(
            "You can edit entries, remove website associations, change preferences, lock the session, turn off activity records, or uninstall the extension at any time.",
            "Deleted accounts first move to the app’s encrypted trash, where they can be restored; that copy is removed once you delete it permanently in the app.",
            "Uninstalling the extension removes its local browser data, but not your vault, app data, exported recovery keys, system backups, clipboard history, or Git remotes. Remove those separately in the app, your file system, and the services you chose. Deleting a local working copy doesn’t delete Git history or remote backups.",
            "Because there are no hosted accounts, the developer cannot access, export, or delete data on your devices, or recover your passphrase or private key.",
          ),
        ],
      },
      changes: {
        title: "Changes to this policy",
        blocks: [
          paragraph(
            "When the way Passpine handles information changes, we’ll update this policy and the date at the top. The policy is kept with the website’s source on GitHub, where earlier versions are available in the commit history.",
          ),
        ],
      },
      contact: {
        title: "Contact",
        blocks: [
          paragraph(
            "General questions about this policy or Passpine can be raised publicly in GitHub Issues. For personal information, privacy requests, or security vulnerabilities, email the maintainer privately.",
          ),
          paragraph(
            "Either way, please never send passwords, private keys, passphrases, codes, vault files, or other sensitive information.",
          ),
        ],
      },
    },
    contactLink: "Go to GitHub Issues",
    contactEmail: (email) => `Email ${email}`,
    backToTop: "Back to top",
  },
  notFound: {
    eyebrow: "404",
    title: "This page doesn’t exist",
    description: "The link may be broken, or the page may have moved.",
    home: "Back to home",
    privacy: "View the privacy policy",
  },
};

export type LanguagePreference = "system" | "en" | "zh-CN";
export function resolveLanguage(
  preference: LanguagePreference,
  languages: readonly string[],
): "en" | "zh-CN" {
  if (preference !== "system") return preference;
  // Match the desktop and extension: explicit scripts take precedence over
  // regions, and unsupported Chinese variants fall back to English.
  const locale =
    (languages[0] ?? "")
      .trim()
      .toLowerCase()
      .replaceAll("_", "-")
      .split(/[.@]/)[0] ?? "";
  const [language, ...parts] = locale.split("-");
  if (language !== "zh") return "en";
  for (const part of parts) {
    if (part === "hans") return "zh-CN";
    if (part.length === 4) return "en";
  }
  return parts.some((part) => ["tw", "hk", "mo"].includes(part))
    ? "en"
    : "zh-CN";
}
