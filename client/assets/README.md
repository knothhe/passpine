# 应用图标

图标与界面保留两套配套风格，在客户端或扩展的设置页切换：

- **陶土橙**：暖白 `#FCFBF9` 圆角底板与 Onevra 默认主色 `#BD481B` 植物钥匙。
- **黑白**：白色 `#FFFFFF` 圆角底板与深色 `#1C1C21` 植物钥匙，搭配参考 racomi / magpie 的冷灰主题。

应用窗口、Dock 和界面品牌使用上述浅色底板。浏览器工具栏使用反转配色：陶土橙或深色底板、纯白植物钥匙。主题风格与跟随系统 / 浅色 / 深色外观独立选择。

工具栏使用独立尺寸布局：底板铺满位图，图形按可见轮廓居中，占画布高度约 72%，避免应用图标的外圈留白叠加浏览器按钮留白。16px 与 32px Retina 资源使用同一简化轮廓；桌面图标保持 macOS 布局。manifest 的 `action.default_icon` 与运行时主题切换使用同一组工具栏资源。

`generate.swift` 中的矢量几何是统一来源，同时生成：

- `app-icon.svg`、`app-icon.png` 和 `Passpine.icns`，以及对应的 `-monochrome` 版本。
- 浏览器 `src/assets/app-icon.png` 和 `app-icon-monochrome.png`，用于界面品牌和页签。
- 浏览器 `public/icon16.png`、`icon32.png`、`icon48.png`、`icon128.png` 和对应的 `-monochrome` 版本，用于扩展工具栏；16px 版本简化枝叶细节。
- `menu-bar-template.png`：32px 黑色前景与透明背景，由 macOS 根据菜单栏外观自动调整前景颜色。

两份 ICNS 均包含完整 macOS 标准与 Retina 尺寸，并保留在应用包内。Dock / 窗口图标在运行时跟随主题风格；Finder 的应用包图标保留默认陶土橙版。其他平台托盘使用当前风格的完整应用图标。

修改几何或配色后，在仓库根目录重新生成全部资源：

```sh
swift -module-cache-path /tmp/passpine-icon-module-cache client/assets/generate.swift
```
