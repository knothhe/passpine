import { createContext, useContext } from "react";

// All on-screen text for the promo and posters lives here, grouped by scene.
// Wording stays aligned with website/src/constants/copy.ts, client/i18n, and browser/src/i18n.
export const zh = {
  locale: "zh-CN" as const,
  brand: "Passpine",
  eyebrow: "开源 · 本地优先 · 兼容 PASS",
  heroTitle: "你的密码库。",
  heroAccent: "你的密钥。",
  description: "用自己的 GPG 密钥加密每条密码，本地保存、浏览器填充、按需 Git 同步。",
  poster: {
    filmBadge: "30 秒产品演示",
    desktopLabel: "桌面客户端",
    extensionLabel: "浏览器扩展",
  },
  fill: {
    eyebrow: "BROWSER COMPANION",
    title: "浏览器一键填充",
    caption: "扩展自动匹配当前网站，\n选中账号即可填入登录框。",
    spec: "Chrome · Edge · Brave · Firefox",
    site: "github.com/login",
    loginTitle: "登录 GitHub",
    emailLabel: "用户名或邮箱",
    passwordLabel: "密码",
    signIn: "登录",
    filled: "已填充",
    username: "pine@example.test",
    extensionTag: "真实扩展界面",
  },
  generate: {
    eyebrow: "STRONG PASSWORDS",
    title: "生成并加密保存",
    caption: "在桌面端新建条目时生成强密码，\n以 OpenPGP 加密写入本地磁盘。",
    spec: "OpenPGP 加密 · 离线可用",
    window: "Passpine — 个人密码库",
    modalTitle: "新建条目",
    nameLabel: "条目路径",
    name: "工作/Figma",
    usernameLabel: "用户名",
    username: "pine@example.test",
    passwordLabel: "密码",
    passwordPlaceholder: "输入或生成强密码",
    generateButton: "生成密码",
    length: "长度",
    lengthValue: "20 位",
    symbols: "包含符号",
    entropy: "高强度 · 128-bit",
    password: "q7#Lw9!zR2m@Xe4$Tn8v",
    fillButton: "填入强密码",
    save: "加密保存",
    saved: "已加密写入 工作/Figma.gpg",
  },
  sync: {
    eyebrow: "YOUR GIT, YOUR CHOICE",
    title: "按需 Git 同步",
    caption: "连接你自己的远端仓库，\n多设备同步条目，私钥始终留在本机。",
    spec: "私钥不进入仓库 · 支持 TOTP",
    window: "Passpine — 个人密码库",
    syncing: "正在同步…",
    synced: "已同步 · 新增 1 个条目",
    gitRemote: "git push origin main",
    totpHeader: "动态验证码 · TOTP",
    totpAccount: "开发 / GitHub",
    totp: "284 913",
    totpRemaining: "24s",
    keyNote: "私钥仅存本机",
  },
  vault: {
    eyebrow: "STANDARD PASS FORMAT",
    title: "原生兼容 pass",
    caption: "已在用 pass？直接导入 ~/.password-store，\n沿用 .gpg 与 .gpg-id 格式不变。",
    spec: "~/.password-store · 无需锁定平台",
    window: "导入 pass 密码库",
    importedWindow: "Passpine — password-store",
    tabs: ["创建密码库", "导入密码库", "从 Git 下载"] as const,
    found: "检测到默认密码库 ~/.password-store，选择密钥来源即可导入。",
    keySource: "密钥来源",
    sources: ["系统 GPG", "密钥文件"] as const,
    files: [
      { name: ".gpg-id", encrypted: false },
      { name: "开发/GitHub.gpg", encrypted: true },
      { name: "开发/Cloudflare.gpg", encrypted: true },
      { name: "工作/Figma.gpg", encrypted: true },
      { name: "生活/Apple.gpg", encrypted: true },
    ] as const,
    importButton: "导入密码库",
    encryptedTag: "OpenPGP",
    importedToast: "已导入 4 个加密条目 · 格式保持不变",
  },
  outro: {
    points: ["无需注册账户", "密码条目本地加密", "标准 pass 格式"] as const,
    platforms: "macOS · Windows · Linux",
    url: "passpine.knothhe.com",
  },
};

export interface Copy {
  locale: "zh-CN" | "en";
  brand: string;
  eyebrow: string;
  heroTitle: string;
  heroAccent: string;
  description: string;
  poster: {
    filmBadge: string;
    desktopLabel: string;
    extensionLabel: string;
  };
  fill: {
    eyebrow: string;
    title: string;
    caption: string;
    spec: string;
    site: string;
    loginTitle: string;
    emailLabel: string;
    passwordLabel: string;
    signIn: string;
    filled: string;
    username: string;
    extensionTag: string;
  };
  generate: {
    eyebrow: string;
    title: string;
    caption: string;
    spec: string;
    window: string;
    modalTitle: string;
    nameLabel: string;
    name: string;
    usernameLabel: string;
    username: string;
    passwordLabel: string;
    passwordPlaceholder: string;
    generateButton: string;
    length: string;
    lengthValue: string;
    symbols: string;
    entropy: string;
    password: string;
    fillButton: string;
    save: string;
    saved: string;
  };
  sync: {
    eyebrow: string;
    title: string;
    caption: string;
    spec: string;
    window: string;
    syncing: string;
    synced: string;
    gitRemote: string;
    totpHeader: string;
    totpAccount: string;
    totp: string;
    totpRemaining: string;
    keyNote: string;
  };
  vault: {
    eyebrow: string;
    title: string;
    caption: string;
    spec: string;
    window: string;
    importedWindow: string;
    tabs: readonly [string, string, string];
    found: string;
    keySource: string;
    sources: readonly [string, string];
    files: readonly { name: string; encrypted: boolean }[];
    importButton: string;
    encryptedTag: string;
    importedToast: string;
  };
  outro: {
    points: readonly [string, string, string];
    platforms: string;
    url: string;
  };
}

export const en: Copy = {
  locale: "en",
  brand: "Passpine",
  eyebrow: "OPEN-SOURCE · LOCAL FIRST · PASS COMPATIBLE",
  heroTitle: "Your vault.",
  heroAccent: "Your keys.",
  description:
    "Encrypt every password with your own GPG key. Fill from your browser and sync with your own Git repo.",
  poster: {
    filmBadge: "30s PRODUCT FILM",
    desktopLabel: "Desktop app",
    extensionLabel: "Browser extension",
  },
  fill: {
    eyebrow: "BROWSER COMPANION",
    title: "Fill sign-ins fast",
    caption: "The extension matches the site.\nPick an account to fill it in place.",
    spec: "Chrome · Edge · Brave · Firefox",
    site: "github.com/login",
    loginTitle: "Sign in to GitHub",
    emailLabel: "Username or email",
    passwordLabel: "Password",
    signIn: "Sign in",
    filled: "Filled",
    username: "pine@example.test",
    extensionTag: "Live extension UI",
  },
  generate: {
    eyebrow: "STRONG PASSWORDS",
    title: "Generate & encrypt",
    caption: "Create strong passwords on desktop,\nencrypted locally with OpenPGP.",
    spec: "OpenPGP encrypted · Offline ready",
    window: "Passpine — Personal vault",
    modalTitle: "New entry",
    nameLabel: "Entry path",
    name: "Work/Figma",
    usernameLabel: "Username",
    username: "pine@example.test",
    passwordLabel: "Password",
    passwordPlaceholder: "Enter or generate a password",
    generateButton: "Generate",
    length: "Length",
    lengthValue: "20 chars",
    symbols: "Symbols",
    entropy: "Strong · 128-bit",
    password: "q7#Lw9!zR2m@Xe4$Tn8v",
    fillButton: "Use password",
    save: "Encrypt & save",
    saved: "Encrypted to Work/Figma.gpg",
  },
  sync: {
    eyebrow: "YOUR GIT, YOUR CHOICE",
    title: "Sync on your terms",
    caption: "Sync encrypted entries via your Git remote.\nYour private key stays on your device.",
    spec: "Keys never synced · Built-in TOTP",
    window: "Passpine — Personal vault",
    syncing: "Syncing…",
    synced: "Synced · 1 new entry",
    gitRemote: "git push origin main",
    totpHeader: "ONE-TIME CODE · TOTP",
    totpAccount: "Development / GitHub",
    totp: "284 913",
    totpRemaining: "24s",
    keyNote: "Private key stays local",
  },
  vault: {
    eyebrow: "STANDARD PASS FORMAT",
    title: "Import a pass vault",
    caption: "Open ~/.password-store as is.\nKeep .gpg and .gpg-id untouched.",
    spec: "~/.password-store · Zero lock-in",
    window: "Import pass vault",
    importedWindow: "Passpine — password-store",
    tabs: ["Create vault", "Import vault", "Clone from Git"],
    found: "Found default vault at ~/.password-store. Choose a key source to import.",
    keySource: "Key source",
    sources: ["System GPG", "Key files"],
    files: [
      { name: ".gpg-id", encrypted: false },
      { name: "Development/GitHub.gpg", encrypted: true },
      { name: "Development/Cloudflare.gpg", encrypted: true },
      { name: "Work/Figma.gpg", encrypted: true },
      { name: "Personal/Apple.gpg", encrypted: true },
    ],
    importButton: "Import vault",
    encryptedTag: "OpenPGP",
    importedToast: "Imported 4 encrypted entries · Format unchanged",
  },
  outro: {
    points: ["No account to create", "Encrypted locally", "Standard pass format"],
    platforms: "macOS · Windows · Linux",
    url: "passpine.knothhe.com",
  },
};

export type Locale = "zh-CN" | "en";
export const dictionaries: Record<Locale, Copy> = { "zh-CN": zh, en };

export const CopyContext = createContext<Copy>(zh);
export const useCopy = () => useContext(CopyContext);
