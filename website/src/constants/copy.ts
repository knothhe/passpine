export const zh = {
  meta: {
    title: "Passpine — 兼容 pass 的本地密码管理器",
    description:
      "兼容 pass 的本地密码管理器。原生桌面客户端、密码生成、TOTP、Git 同步与浏览器填充。下载 macOS、Windows 和 Linux 版本。",
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
    note: "本地优先。由你掌握。",
  },
};

export type Copy = typeof zh;

export const en: Copy = {
  meta: {
    title: "Passpine — A local password manager compatible with pass",
    description:
      "A local password manager compatible with pass. Native desktop apps, password generation, TOTP, Git sync, and browser filling. Download for macOS, Windows, and Linux.",
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
    note: "Local first. Yours to keep.",
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
