# 应用图标

`Passpine.icns` 是应用图标的统一来源，沿用原生 Passpine 客户端的叶片图形。

`app-icon.png`、浏览器的 `src/assets/app-icon.png` 以及 `public/icon16.png`、`icon32.png`、`icon48.png`、`icon128.png` 均由同一份图标导出。应用窗口、Dock、浏览器工具栏、设置页和页签采用这些资源。

`menu-bar-template.png` 使用原有客户端相同的绿色前景提取方式，保留叶片与茎的轮廓，去除图标底板。macOS 根据菜单栏外观自动调整前景颜色；其他平台托盘使用完整应用图标。

修改 `Passpine.icns` 后，在仓库根目录重新生成：

```sh
swift -module-cache-path /tmp/passpine-icon-module-cache client/assets/generate.swift
```
