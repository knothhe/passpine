# 共享资产

仓库级图片资产集中在这里：

| 目录 | 内容 | 生成方式 |
| --- | --- | --- |
| `website/<zh-CN\|en>/` | 官网首页的客户端与扩展截图 | `scripts/capture-website-screenshots.mjs`，见 [官网说明](../website/README.md#产品截图) |
| `chrome-web-store/` | Chrome 应用商店图标、宣传图与中英文截图；`source-screenshots/` 为合成前的原始弹窗 | `scripts/generate-store-art.mjs`、`scripts/capture-store-screenshots.mjs`，见 [素材记录](../docs/chrome-web-store/assets.md) |
| `screenshots/` | 文档引用的截图 | 手动拍摄 |

应用图标仍保留在各自的构建目录：`client/assets` 由 Go `embed` 打包进客户端（`embed` 不能引用模块目录以外的文件），`browser/public` 与 `browser/src/assets` 是 WXT/Vite 构建输入，三者均由 `client/assets/generate.swift` 统一生成。`design/` 下的截图是设计过程记录，随各自设计稿保存。
