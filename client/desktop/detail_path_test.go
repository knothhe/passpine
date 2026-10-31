package desktop

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/i18n"
)

func TestDetailCopiesFullEntryPathAcrossLanguagesAndWidths(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		for _, path := range []string{"GitHub", "设置/密码/账户 with spaces", strings.Repeat("long-folder/", 12) + "账户"} {
			t.Run(language+"/"+path, func(t *testing.T) {
				a := fixture()
				a.state.Settings.Language = language
				a.selected, a.detail.Entry = path, path
				a.service = backendFunc(func(app.Request) app.Response {
					t.Error("copying an entry path requested credentials")
					return app.Response{}
				})
				var copied string
				a.clipboardWriter = func(value string, sensitive bool) error {
					if sensitive {
						t.Error("entry path was marked as a secret")
					}
					copied = value
					return nil
				}
				view := ui.NewTester(testView(a), 1120, 760)
				for _, width := range []int{1120, 860} {
					view.Move(0, 0)
					view.SetSize(width, 760)
					// Let the previous success toast expire before clicking the
					// toolbar at its new position after resizing.
					view.Advance(4 * time.Second)
					copyBounds, found := view.Find(a.t("复制条目完整路径"))
					editBounds, editFound := view.Find(a.t("编辑条目"))
					if !found || !editFound || copyBounds.W <= 0 || copyBounds.X < defaultSidebarWidth || copyBounds.X+copyBounds.W > editBounds.X {
						t.Fatalf("path copy control is missing or overlaps the toolbar: copy=%v edit=%v", copyBounds, editBounds)
					}
					copied = ""
					if err := view.Click(a.t("复制条目完整路径")); err != nil {
						t.Fatal(err)
					}
					if copied != path || !view.HasText(a.t("条目完整路径已复制")) {
						t.Fatalf("full entry path was changed or feedback is missing at width %d: got %q, want %q", width, copied, path)
					}
					if a.clipboardTimer != nil || a.busy || view.HasText(a.detail.Password) {
						t.Fatal("copying metadata started a credential operation, scheduled expiry or revealed a secret")
					}
				}
			})
		}
	}
}

func TestDetailPathCopyReportsClipboardFailure(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		t.Run(language, func(t *testing.T) {
			a := fixture()
			a.state.Settings.Language = language
			a.clipboardWriter = func(string, bool) error { return errors.New("clipboard unavailable") }
			view := ui.NewTester(testView(a), 860, 620)
			if err := view.Click(a.t("复制条目完整路径")); err != nil {
				t.Fatal(err)
			}
			if !view.HasText(a.t(i18n.ClipboardWriteFailed)) || a.notice != "" {
				t.Fatal("clipboard failure was not reported accurately")
			}
		})
	}
}
