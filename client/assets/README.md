# 应用图标

客户端与扩展默认使用黑白（monochrome）图标和主题；保留两套配套风格，在设置页切换：

- **陶土橙**：暖白渐变圆角底板与 Onevra 默认主色 `#BD481B` 松鼠剪影。
- **黑白**：白色至浅灰 `#EFEFEF` 的圆角底板与深色 `#171918` 松鼠剪影。

应用窗口、Dock 和界面品牌使用上述浅色底板，在深浅背景下均保持清晰边界。浏览器工具栏使用反转配色：陶土橙或深色底板、纯白松鼠。主题风格与跟随系统 / 浅色 / 深色外观独立选择。

工具栏使用独立尺寸布局：底板铺满位图，避免应用图标的外圈留白叠加浏览器按钮留白。16px 与 32px Retina 使用 16px 专用轮廓，24px 与 48px Retina 使用 24px 专用轮廓，保留较大的卷尾和胸腹留白。大尺寸使用完整主稿。桌面图标保持 macOS 留白，小尺寸 ICNS 也使用对应的简化轮廓。manifest 的 `action.default_icon` 与运行时主题切换使用相同的完整尺寸映射。

`icon-artwork.json` 是主稿、16px 与 24px 光学校正轮廓的统一矢量来源，取自已确认的流线松鼠方案。`generate.swift` 使用同一份路径及负形数据导出 SVG 并通过 Core Graphics 栅格化，无额外图形依赖，同时生成：

- `app-icon.svg`、`app-icon.png` 和 `Passpine.icns`，以及对应的 `-monochrome` 版本。
- 浏览器 `src/assets/app-icon.png` 和 `app-icon-monochrome.png`，用于界面品牌和页签。
- 浏览器 `public/icon16.png`、`icon24.png`、`icon32.png`、`icon48.png`、`icon128.png` 和对应的 `-monochrome` 版本，用于扩展工具栏与管理页。
- `menu-bar-template.png`：32px 黑色前景与透明背景，由 macOS 根据菜单栏外观自动调整前景颜色。

两套 ICNS 资源均包含完整 macOS 标准与 Retina 尺寸。Dock 在运行时直接加载完整 ICNS，保留小尺寸光学校正与多分辨率表示；窗口与其他平台使用 PNG。图标在运行时跟随主题风格；构建时 Finder 的默认应用包图标使用黑白版。其他平台托盘使用当前风格的完整应用图标。

修改几何或配色后，在仓库根目录重新生成全部资源：

```sh
swift -module-cache-path /tmp/passpine-icon-module-cache client/assets/generate.swift
```
