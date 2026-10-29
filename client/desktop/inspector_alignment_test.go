package desktop

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestEditorPasswordActionsShareTheInputCenter(t *testing.T) {
	for _, scale := range []int{80, 100, 120} {
		t.Run(fmt.Sprintf("scale-%d", scale), func(t *testing.T) {
			a := fixture()
			a.state.Settings.FontScale = scale
			a.editEntry()
			view := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.clientTheme(c))
				ui.Column(c).Fill().Background(c.Theme().Surface).Children(func() { a.editorView(c) })
			}, 580, 700)
			eye, eyeFound := view.Find("显示或隐藏编辑密码")
			generator, generatorFound := view.Find("生成并填入密码")
			caption, captionFound := view.Find("密码")
			if !eyeFound || !generatorFound || !captionFound {
				t.Fatal("password field or its actions are missing")
			}
			if math.Abs(float64(eye.Y+eye.H/2-generator.Y-generator.H/2)) > .5 {
				t.Fatal("reveal and generator actions are not centered on the same row")
			}
			// Find the actual painted field at an empty position near its right
			// edge. This catches a large icon target aligned to the field's bottom
			// while leaving the input's native text metrics and padding intact.
			image, background := view.Image(), monochromeInputBackground(false)
			x := int(eye.X - 16)
			first, last := -1, -1
			for y := int(math.Ceil(float64(caption.Y + caption.H))); y < int(eye.Y+eye.H+6); y++ {
				pixel := image.RGBAAt(x, y)
				if pixel.R == background.R && pixel.G == background.G && pixel.B == background.B {
					if first < 0 {
						first = y
					}
					last = y
				}
			}
			if first < 0 || last-first < 18 {
				t.Fatal("password input's painted surface is missing")
			}
			inputCenter := float32(first+last+1) / 2
			if math.Abs(float64(eye.Y+eye.H/2-inputCenter)) > 1 {
				t.Fatalf("password actions are vertically offset from the field: action %.1f, field %.1f", eye.Y+eye.H/2, inputCenter)
			}
		})
	}
}

func TestEditorAndRecoveryHeadingsShareTheContentGrid(t *testing.T) {
	for _, width := range []int{580, 900} {
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			a := recoveryColumnsFixture()
			a.editEntry()
			editor := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.clientTheme(c))
				a.editorView(c)
			}, width, 620)
			recovery := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.clientTheme(c))
				a.recoveryDetailView(c, a.recovery[0])
			}, width, 620)
			editorTitle, editorFound := editor.Find("编辑条目")
			recoveryTitle, recoveryFound := recovery.Find("First")
			close, closeFound := editor.Find("关闭编辑器")
			purge, purgeFound := recovery.Find("永久删除恢复副本")
			if !editorFound || !recoveryFound || !closeFound || !purgeFound {
				t.Fatal("inspector heading or action is missing")
			}
			if editorTitle.X != recoveryTitle.X || close.X != purge.X || close.Y != purge.Y {
				t.Fatalf("editor and recovery use different heading grids: editor %v/%v, recovery %v/%v", editorTitle, close, recoveryTitle, purge)
			}
			for _, pair := range [][2]ui.Rect{{editorTitle, close}, {recoveryTitle, purge}} {
				if math.Abs(float64(pair[0].Y+pair[0].H/2-pair[1].Y-pair[1].H/2)) > .5 {
					t.Fatal("inspector title and action do not share a vertical center")
				}
			}
		})
	}
}

func TestRecoveryCountUsesTheFixedListFooter(t *testing.T) {
	a := recoveryColumnsFixture()
	view := ui.NewTester(recoveryColumnsTestView(a, 300), 900, 620)
	count, found := view.Find(a.countLabel(len(a.recovery)))
	if !found || count.X != pageGutter || count.Y < 581 || count.Y+count.H > 620 {
		t.Fatalf("recovery count is not aligned with the fixed list footer: %v", count)
	}
	view.Scroll(120, 180, 0, 400)
	if after, ok := view.Find(a.countLabel(len(a.recovery))); !ok || after != count {
		t.Fatal("scrolling recovery items moved the list count")
	}
}
