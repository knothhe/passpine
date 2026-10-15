# PassPine Neo 浏览器伴侣

WXT + Vite + TypeScript + React，使用 Radix Themes 可访问组件与 Lucide 图标。颜色、圆角与原生客户端的 Onevra 主题一致；不在扩展内嵌入 WebView 客户端，也不包含 Rust。

工具栏、界面品牌和页签图标统一从客户端 `client/assets/Passpine.icns` 导出，生成方式见 [应用图标说明](../client/assets/README.md)。界面控件使用 Radix Themes，功能操作图标使用 Lucide，配色来源为 Onevra 的公开主题。

## 开发与构建

```sh
npm ci
npm run typecheck
npm test
make -C .. browser:build
npm run zip
```

[本地安装、配置与浏览器加载说明](../docs/install-local.md)

客户端设置中的「浏览器连接」负责注册本机 Native Messaging Host。扩展身份固定为：

- Chrome：`obnlebeedcbbchadakacilopfdaifgjh`
- Firefox：`passpine-neo@knothhe.com`
- Native host：`com.knothhe.passpine.neo`

Chrome manifest 的 `key` 是本仓库生成的公钥，仅用于固定开发版 ID；没有保留私钥。

## 独立交互预览

```sh
npm run preview
```

打开 `http://127.0.0.1:5174/`。这是显式 demo 入口，使用独立的虚构数据，不连接 Native Messaging，不读真实密码库。真实扩展的 entrypoints 不导入 demo client；连接失败时只呈现连接失败状态，不会回退到示例数据。

## 组件边界

`src/components/AccountList.tsx` 负责可复用账号列表，`Primitives.tsx` 包含品牌、账号头像、操作按钮及凭证字段；`EditorForm.tsx`、`Generator.tsx`、`ConnectionState.tsx` 分别实现编辑、生成器和连接状态。`App.tsx` 只负责弹窗状态与业务编排；`Options.tsx` 负责连接指引。新建、编辑和生成器直接在扩展弹窗内切换页面，使用统一导航、滚动内容和固定操作栏；设置页采用全宽响应式布局。

列表采用连续排列的紧凑布局，网站路径在条目名称上方，当前网站匹配条目优先并显示行内标记，不额外分组。每个条目依次提供用户名、密码、详情、更多操作，使用 Lucide 图标和当前主题色悬停效果。锁定时默认保留名称和本地搜索；显式读取、复制或填充会自动请求客户端的系统验证弹窗，成功后继续原操作。未配置系统解锁时会唤出客户端；名称列表和 OTP 定时刷新不会触发验证。

## 通信与安全边界

- 后台通过持久 `runtime.connectNative` Port 连接 Go host。`requestId` 关联并发响应，复用进程与本机连接。只合并在途 `configure` 请求；断线后拒绝所有在途请求，下次显式操作才重连。保存、删除、复制从不自动重放。
- 协议为 `{ protocol: 1, requestId, action, ... }`，返回 `{ protocol: 1, requestId, status, data, code?, message? }`。所有条目操作携带当前 `vaultId`；编辑和删除携带读取时的 `revision`，重命名使用原条目加 `destination`。
- 只允许正确扩展 ID 和精确 `popup.html` / `options.html` 地址发起内部请求；设置页允许浏览器标签页元数据。无后台响应、格式异常与真实协议版本差异分别报告，避免将连接失败误报为版本不一致。内容脚本和任意网页不能请求密码。
- 权限仅为 `activeTab`、`scripting`、`nativeMessaging`、`storage`、`clipboardWrite`。没有 `<all_urls>`、`tabs`、`clipboardRead` 或常驻注入脚本。
- 用户点击填充或使用快捷键后，后台核验活动 tab ID 与完整 URL、账号关联的精确 hostname，再向顶层 frame 注入一次性填写函数；函数再次核验完整 URL。不会自动提交表单，也不进入跨域 iframe。
- 系统验证期间暂停状态轮询，并将排队的元数据请求延后到验证结束。弹窗可见时每 2 秒读取轻量 status，只有 entriesVersion 改变才刷新完整列表。锁定、切库、断线或页面隐藏会清除详情与编辑状态；会话 epoch 和响应序号防止过期解密结果重新显示。OTP 到期先遮挡，再在仍打开的详情中刷新。
- 密码与私钥不写入浏览器持久存储。网站关联仅以 vault/entry/hostname 组合的 SHA-256 摘要存储；不使用远程 favicon 请求。条目复制由原生客户端操作剪贴板并按客户端设置清空。
- 独立生成器的「复制」使用浏览器剪贴板，生成值仅保存在打开的 UI 内存中，不会自动保存或自动清空剪贴板。

## 当前覆盖与限制

支持全库搜索、当前网站优先、锁定/断线状态、详情、用户名/密码/OTP 复制、生成密码、新建、编辑与改名、加密回收站删除、精确网站关联、键盘快捷键和连接指引。保留编辑条目的自定义字段、OTP 配置与附加 URL。

自动填充针对顶层页面中的标准 HTML 登录字段。跨域 iframe、Shadow DOM、自定义编辑器、HTTP Basic Auth 与通行密钥不在此版填充范围；可改用复制。回收站恢复、私钥管理与 Git 同步在原生客户端完成。默认不跨子域继承关联，子域需要单独关联。

视觉检查覆盖列表、详情、新建、编辑及错误、生成器、空库、空搜索、未配置、未连接、锁定、设置和确认框；补齐 Radix 弹出层的亮色 / 暗色主题变量，避免透明背景。

测试覆盖 Native Port 复用与断线不重放、发送者边界、协议字段白名单、域名伪装、路径验证、编辑保留字段、全库搜索，以及真实 DOM 的字段选择、事件触发、不提交和 URL/frame 核验。

验证边界：已运行 TypeScript 检查、纯逻辑/DOM 回归测试和 Chrome/Firefox 构建，并通过内置浏览器验收显式 demo 界面。尚未将扩展加载到用户的真实 Chrome/Firefox 配置进行完整 Native Messaging + 真实网站端到端验收。
