const quickSearchCopy = {
  'zh-CN': {
    username: (name) => `复制用户名 ${name}`,
    password: (name) => `复制密码 ${name}`,
    details: (name) => `查看条目 ${name}`,
  },
  en: {
    username: (name) => `Copy username for ${name}`,
    password: (name) => `Copy password for ${name}`,
    details: (name) => `View entry ${name}`,
  },
};
globalThis.PASSPINE_QUICK_SEARCH_COPY = quickSearchCopy;
