package i18n

const (
	LayoutAuditTitle                = "布局走查"
	LayoutAuditOutlines             = "边界"
	LayoutAuditGrid                 = "8 DIP 网格"
	LayoutAuditRuler                = "两点测距"
	LayoutAuditClear                = "清除测距"
	LayoutAuditCollapse             = "收起"
	LayoutAuditExpand               = "展开走查"
	LayoutAuditPickFirst            = "点击第一个测量点。按 Esc 退出测距。"
	LayoutAuditPickSecond           = "点击第二个测量点。测距点击不会操作下方控件。"
	LayoutAuditComplete             = "再次点击可开始新的测量。"
	LayoutAuditLegend               = "蓝色为边界，绿色为内容内边界，橙色为外边距。手动测距使用窗口逻辑单位 DIP。"
	LayoutAuditInspector            = "F12 打开 MyGo 组件检查器：选择元素查看盒模型、字体、间距与源码。检查器自身目前使用 English。"
	LayoutAuditInspectorUnavailable = "此构建未包含 MyGo 组件检查器；请使用不带 mygo_noinspector 标签的开发构建。"
	LayoutAuditViewport             = "当前画布：%.0f × %.0f DIP"
	LayoutAuditDistance             = "水平 %.1f · 垂直 %.1f DIP"
	LayoutAuditMenu                 = "原生组件检查器"
	LayoutAuditFlag                 = "预览模式：显示原生布局边界、参考网格与两点测距工具"
	LayoutAuditRequiresPreview      = "布局走查必须同时使用 --preview；仅检查隔离的虚构数据。"
	LayoutAuditTemporaryData        = "布局走查会自动创建临时密码库，请移除 --data-dir。"
	LayoutAuditNoSeed               = "布局走查不能同时使用 --seed-test-vault；该操作会写入开发版应用数据目录。"
)

func init() {
	for key, value := range map[string]string{
		LayoutAuditTitle: "Layout audit", LayoutAuditOutlines: "Bounds", LayoutAuditGrid: "8 DIP grid", LayoutAuditRuler: "Ruler",
		LayoutAuditClear: "Clear ruler", LayoutAuditCollapse: "Collapse", LayoutAuditExpand: "Expand audit",
		LayoutAuditPickFirst:            "Click the first point. Press Esc to leave ruler mode.",
		LayoutAuditPickSecond:           "Click the second point. Ruler clicks do not activate controls underneath.",
		LayoutAuditComplete:             "Click again to start a new measurement.",
		LayoutAuditLegend:               "Blue: bounds. Green: inner content edge. Orange: margins. Manual measurements use window DIPs.",
		LayoutAuditInspector:            "F12 opens the MyGo inspector: pick an element for box model, font, spacing and source. Its own interface currently uses English.",
		LayoutAuditInspectorUnavailable: "This build excludes the MyGo inspector. Use a development build without mygo_noinspector.",
		LayoutAuditViewport:             "Current canvas: %.0f × %.0f DIP", LayoutAuditDistance: "Horizontal %.1f · Vertical %.1f DIP",
		LayoutAuditMenu: "Native component inspector", LayoutAuditFlag: "Preview mode: show native layout bounds, reference grid and a two-point ruler",
		LayoutAuditRequiresPreview: "Layout audit requires --preview and only inspects isolated fictional data.",
		LayoutAuditTemporaryData:   "Layout audit creates its own temporary vault. Remove --data-dir.",
		LayoutAuditNoSeed:          "Layout audit cannot use --seed-test-vault, which writes to the development application's data directory.",
	} {
		english[key] = value
	}
}
