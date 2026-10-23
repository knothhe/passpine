package desktop

import (
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func visualBounds(t *testing.T, view *ui.Tester, label string) ui.Rect {
	t.Helper()
	bounds, found := view.Find(label)
	if !found || bounds.W <= 0 || bounds.H <= 0 {
		t.Fatalf("visible element %q is missing: %v", label, bounds)
	}
	return bounds
}

func visualNear(a, b float32) bool { return math.Abs(float64(a-b)) <= .5 }

// The selected username and folder also appear in the list and breadcrumb.
// Check the text in its intended column instead of taking the first duplicate.
func visualBoundsIn(t *testing.T, view *ui.Tester, text string, container ui.Rect) ui.Rect {
	t.Helper()
	labels := reflect.ValueOf(view).Elem().FieldByName("rt").Elem().FieldByName("labels")
	for i := range labels.Len() {
		label := labels.Index(i)
		if label.FieldByName("text").String() != text {
			continue
		}
		bounds := label.FieldByName("r")
		r := ui.Rect{X: float32(bounds.FieldByName("X").Float()), Y: float32(bounds.FieldByName("Y").Float()), W: float32(bounds.FieldByName("W").Float()), H: float32(bounds.FieldByName("H").Float())}
		if r.W > 0 && r.H > 0 && r.X >= container.X && r.Y >= container.Y && r.X+r.W <= container.X+container.W+.5 && r.Y+r.H <= container.Y+container.H+.5 {
			return r
		}
	}
	t.Fatalf("visible element %q is missing inside %v", text, container)
	return ui.Rect{}
}

func TestManagerVisualSurfacesShareThemeRoles(t *testing.T) {
	for _, style := range []string{app.ThemeStyleMonochrome, app.ThemeStyleOnevra} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dark=%t", style, dark), func(t *testing.T) {
				a := fixture()
				a.state.Settings.ThemeStyle = style
				view := ui.NewTester(testView(a), 1120, 760)
				view.SetDark(dark)
				view.Advance(time.Second)
				palette := clientPalette(style, dark)
				for _, sample := range []struct {
					name string
					x, y int
					want ui.Color
				}{
					{"navigation", 100, 550, palette.Background},
					{"header", 600, 24, palette.Background},
					{"list", 300, 550, palette.Surface},
					{"detail", 1000, 700, palette.Surface},
				} {
					pixel := view.Image().RGBAAt(sample.x, sample.y)
					if pixel.R != sample.want.R || pixel.G != sample.want.G || pixel.B != sample.want.B || pixel.A != 255 {
						t.Errorf("%s uses a different surface: got %v, want %v", sample.name, pixel, sample.want)
					}
				}
			})
		}
	}
}

func TestManagerVisualNavigationFooterSharesLabelColumn(t *testing.T) {
	for _, scale := range []int{80, 120} {
		t.Run(fmt.Sprintf("scale=%d", scale), func(t *testing.T) {
			a := fixture()
			a.state.Settings.FontScale = scale
			// A separate scope title keeps navigation labels unique without
			// changing the actual sidebar and fixed toolbar builders.
			a.filter = "folder:visual-alignment-fixture"
			view := ui.NewTester(testView(a), 1120, 760)
			// These are unique visible texts: their accessible row labels are
			// "文件夹 开发" and "加密草稿", respectively.
			folderRow := visualBounds(t, view, a.t("文件夹 %s", "开发"))
			folder := visualBoundsIn(t, view, "开发", folderRow)
			draft := visualBounds(t, view, a.t("草稿"))
			if !visualNear(folder.X, draft.X) {
				t.Fatalf("fixed footer label drifted from the folder column: folder=%v draft=%v", folder, draft)
			}
			// Repeated text/accessible labels resolve to their outer row. Check
			// the hit targets as well as the unique visible label positions.
			for _, label := range []string{a.t("全部条目"), a.t("加密草稿"), a.t("回收站"), a.t("设置")} {
				row := visualBounds(t, view, label)
				if !visualNear(row.X, folderRow.X) || !visualNear(row.W, folderRow.W) {
					t.Errorf("navigation hit target %q does not share the folder column: folder=%v row=%v", label, folderRow, row)
				}
			}
		})
	}
}

func TestManagerVisualDetailLabelsAndActionsAlignAtWindowAndTextSizes(t *testing.T) {
	for _, width := range []int{860, 1120} {
		for _, scale := range []int{80, 120} {
			for _, language := range []string{"zh-CN", "en"} {
				t.Run(fmt.Sprintf("width=%d/scale=%d/%s", width, scale, language), func(t *testing.T) {
					a := fixture()
					a.state.Settings.FontScale, a.state.Settings.Language = scale, language
					view := ui.NewTester(testView(a), width, 620)
					password := visualBounds(t, view, a.t("密码"))
					detailFields := visualBounds(t, view, a.t("账号字段"))
					passwordCopy := visualBounds(t, view, a.t("复制%s", a.t("密码")))
					for _, label := range []string{"用户名", "网址", "备注"} {
						field := visualBounds(t, view, a.t(label))
						if !visualNear(field.X, password.X) {
							t.Errorf("field label %q drifted from the password label: password=%v field=%v", label, password, field)
						}
					}
					for _, field := range []struct{ label, value string }{
						{"密码", "••••••••••••"},
						{"用户名", a.detail.Username},
						{"网址", a.detail.URL},
					} {
						copy := visualBounds(t, view, a.t("复制%s", a.t(field.label)))
						value := visualBoundsIn(t, view, field.value, detailFields)
						if !visualNear(copy.X+copy.W, passwordCopy.X+passwordCopy.W) {
							t.Errorf("copy action %q has a different right edge: password=%v copy=%v", field.label, passwordCopy, copy)
						}
						if !visualNear(copy.Y+copy.H/2, value.Y+value.H/2) {
							t.Errorf("copy action %q is not centered beside its value: value=%v copy=%v", field.label, value, copy)
						}
						if copy.X+copy.W > float32(width) || value.X < 0 || value.X+value.W > copy.X+.5 {
							t.Errorf("field %q overlaps an action or escapes the window: value=%v copy=%v", field.label, value, copy)
						}
					}
				})
			}
		}
	}
}
