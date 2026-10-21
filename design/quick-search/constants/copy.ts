const quickSearchCopy = {
  'zh-CN': {
    username: (name: string) => `复制用户名 ${name}`,
    password: (name: string) => `复制密码 ${name}`,
    details: (name: string) => `查看条目 ${name}`,
  },
  en: {
    username: (name: string) => `Copy username for ${name}`,
    password: (name: string) => `Copy password for ${name}`,
    details: (name: string) => `View entry ${name}`,
  },
};
export default quickSearchCopy;
