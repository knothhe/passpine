# UI 文案

- UI 文案必须集中到 `constants/copy.ts` 或现有 i18n 字典，禁止在 JSX 中硬编码用户可见字符串。
- 浏览器扩展及预览页使用 `browser/src/constants/copy.ts`（简体中文）和 `browser/src/i18n/en.ts`（English），通过 `useCopy()` 获取当前文案；浏览器管理界面的元数据使用 `browser/public/_locales` 字典。按功能分组并使用语义化键名，新增消息须同时补齐两种语言。
- 扩展与客户端支持相同的 `system`、`en`、`zh-CN` 语言选项，跟随系统及不支持语言回退到 English 的规则保持一致。桌面客户端沿用 `client/i18n` 字典和 `a.t(...)`。
- 覆盖正文、标题、按钮、菜单、占位符、Tooltip、`aria-label`、空状态、校验错误、通知、主题说明和快捷键说明。组件外生成的 UI 提示也必须使用统一文案源。
- 含名称、数量、域名等变量的文案使用字典中的完整模板或带类型参数的函数，不在 JSX 或业务逻辑中拼接自然语言片段。
- 协议值、路由、CSS 类名、DOM ID、技术属性、空字符串以及用户数据不属于 UI 文案；密码库内容、账号名称和用户输入不得当作翻译键。
- 修改文案后运行浏览器端类型检查、现有测试和构建，确认两种语言的文案、可访问名称及相关交互仍正常。切换语言不得重启凭证请求、清空编辑输入或翻译用户数据。

# macOS 浏览器验证

- 浏览器验证优先使用内置 Browser 或已连接的 Chrome 工具。
- 必须通过 shell 启动 Chrome、Chromium、Chrome for Testing，或运行会创建浏览器的 Playwright/Puppeteer 测试时，首次就请求沙箱外执行（`sandbox_permissions="require_escalated"`），不要先在默认沙箱内尝试。
- 若出现 Sandbox restriction，或 RegisterApplication / TransformProcessType 阶段的 SIGABRT，停止用相同方式重试，不要并发重启失败的浏览器。
