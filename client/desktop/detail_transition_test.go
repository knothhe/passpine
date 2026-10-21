package desktop

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestDetailTransitionPreservesReadOnlyDisplayWithoutCredentials(t *testing.T) {
	a := fixture()
	a.detail.OTP = "123456"
	a.detail.Contents += "recovery: synthetic-recovery-value\n"
	a.reveal, a.deleteOpen = true, true
	a.revealedFields = map[string]bool{"password": true}
	values := []string{a.detail.Password, a.detail.Contents, a.detail.OTP, "synthetic-recovery-value"}
	original := a.detail
	a.busy, a.backgroundBusy = true, true
	a.selectEntry("工作/Figma")
	presentation := a.detailTransitionPresentation(a.detailLoadingStarted)
	if !presentation.Pending || presentation.Delayed || presentation.Entry != a.selected || !presentation.HasNotes || !presentation.HasWebsite {
		t.Fatalf("entry selection discarded the detail structure: %+v", presentation)
	}
	want := []detailDisplayField{
		{Key: "password", Label: "密码", Secret: true}, {Key: "username", Label: "用户名", Value: original.Username},
		{Key: "url", Label: "网址", Value: original.URL}, {Key: "otp", Label: "一次性密码", Secret: true},
		{Key: "custom:0", Label: "recovery", Secret: true},
	}
	if !reflect.DeepEqual(presentation.Fields, want) || !reflect.DeepEqual(a.detail, app.Data{}) || a.reveal || len(a.revealedFields) != 0 || a.deleteOpen {
		t.Fatal("transition retained decrypted detail or active detail actions")
	}
	if presentation.DisplayEntry != original.Entry || presentation.Notes != original.Notes {
		t.Fatal("loading mixed the target heading with the previous presentation")
	}
	snapshot := fmt.Sprintf("%+v %+v", a.detailTransition, presentation)
	for _, value := range values {
		if strings.Contains(snapshot, value) {
			t.Fatalf("transition snapshot retained the previous entry value %q", value)
		}
	}
	a.selectEntry("开发/Cloudflare")
	next := a.detailTransitionPresentation(a.detailLoadingStarted)
	if next.Entry != "开发/Cloudflare" || !reflect.DeepEqual(next.Fields, want) || !next.HasNotes || !next.HasWebsite {
		t.Fatal("rapid selection collapsed the placeholder before the final read")
	}
}

func TestDetailTransitionDefersLocalFeedbackWithoutRestartingQueuedRead(t *testing.T) {
	a := fixture()
	a.busy, a.backgroundBusy = true, true
	a.selectEntry("工作/Figma")
	started, serial := a.detailLoadingStarted, a.detailSelectionSerial
	a.beginDetailTransition("工作/Figma") // Dispatching the queued read keeps its original delay.
	if !a.detailLoadingStarted.Equal(started) || serial != a.detailSelectionSerial {
		t.Fatal("dispatching a queued selection restarted its feedback delay or lease")
	}
	for _, elapsed := range []time.Duration{0, detailLoadingDelay - time.Millisecond, detailLoadingDelay, time.Second} {
		p := a.detailTransitionPresentation(started.Add(elapsed))
		if !p.Pending || p.Delayed != (elapsed >= detailLoadingDelay) || p.Remaining < 0 {
			t.Fatalf("incorrect local feedback after %v: %+v", elapsed, p)
		}
	}
}

func TestDetailTransitionClearsAcrossSecretAndNavigationBoundaries(t *testing.T) {
	for _, boundary := range []string{"detail", "session", "navigation", "setup", "fetch-error"} {
		t.Run(boundary, func(t *testing.T) {
			a := fixture()
			a.busy, a.backgroundBusy = true, true
			a.selectEntry("工作/Figma")
			before := a.detailSelectionSerial
			switch boundary {
			case "detail":
				a.dropDetail()
			case "session":
				a.clearSecrets()
			case "navigation":
				a.navigate("activity", "")
			case "setup":
				a.openSetup()
			case "fetch-error":
				a.pendingSelection, a.pendingSelectionVaultID = "", ""
				a.activeRequestAction = "fetch"
				a.receiveResponse(a.generation, app.Response{Status: "error", Message: "synthetic failure", Data: a.state}, nil)
			}
			if a.detailTransition.entry != "" || len(a.detailTransition.fields) != 0 || a.detailTransition.displayEntry != "" || a.detailTransition.notes != "" || a.detailTransition.unlocking || !a.detailLoadingStarted.IsZero() || a.detailSelectionSerial <= before || a.detailTransitionPresentation(time.Now()).Pending {
				t.Fatal("invalidated transition retained its structure or request lease")
			}
		})
	}
}

func TestDetailFetchRejectsEarlierVisitToSameEntry(t *testing.T) {
	a := fixture()
	state := a.state
	callbacks := make(chan func(), 2)
	requests := make(chan app.Request, 2)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		requests <- request
		data := state
		data.Entry, data.Password = request.Entry, "synthetic-password"
		return app.Response{Status: "ok", Data: data}
	})
	a.selectEntry("工作/Figma")
	initialSerial := a.detailSelectionSerial
	// A direct navigation invalidates the request even when its destination
	// later has exactly the same selected path.
	a.openSetup()
	a.navigate("entries", "all")
	a.selectEntry("工作/Figma")
	if a.detailSelectionSerial == initialSerial || a.pendingSelection != "工作/Figma" {
		t.Fatal("returning to the same entry reused the earlier visit's request lease")
	}
	awaitMain(t, callbacks)
	if a.detail.Entry != "" || !a.busy || !a.detailTransitionPresentation(time.Now()).Pending {
		t.Fatal("the earlier visit populated the detail before the current read")
	}
	awaitMain(t, callbacks)
	first, second := <-requests, <-requests
	if first.Entry != second.Entry || a.detail.Entry != a.selected || a.detail.Password != "synthetic-password" || a.detailTransition.entry != "" || !a.detailLoadingStarted.IsZero() {
		t.Fatal("the current visit did not replace its placeholder and clear transition state")
	}
}

func TestBusyHistorySelectionPreservesPendingDetailStructure(t *testing.T) {
	a := fixture()
	a.service = backendFunc(func(app.Request) app.Response { t.Fatal("history dispatched a read while busy"); return app.Response{} })
	a.busy, a.backgroundBusy = true, true
	a.selectEntry("工作/Figma")
	previous := a.detailTransitionPresentation(a.detailLoadingStarted)
	serial := a.detailSelectionSerial
	a.restoreLocation(navigationLocation{page: "entries", selected: "开发/Cloudflare", vaultID: a.state.VaultID})
	presentation := a.detailTransitionPresentation(a.detailLoadingStarted)
	if !presentation.Pending || presentation.Entry != "开发/Cloudflare" || !reflect.DeepEqual(presentation.Fields, previous.Fields) || presentation.HasNotes != previous.HasNotes || presentation.HasWebsite != previous.HasWebsite || a.detailSelectionSerial <= serial {
		t.Fatalf("busy history selection lost its detail structure or reused the cancelled lease: %+v", presentation)
	}
}

func TestDetailTransitionKeepsControlsInPlaceAndInactive(t *testing.T) {
	for _, size := range [][2]int{{860, 620}, {1120, 760}} {
		for _, long := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/long=%t", size[0], size[1], long), func(t *testing.T) {
				a := fixture()
				if long {
					a.detail.Username = strings.Repeat("synthetic-account-name-", 10) + "@example.test"
					a.detail.Notes = strings.Repeat("详细备注保留阅读位置。", 10)
				}
				original, state := a.detail, a.state
				callbacks := make(chan func(), 1)
				a.runOnMain = func(callback func()) { callbacks <- callback }
				a.service = backendFunc(func(request app.Request) app.Response {
					if request.Action != "fetch" {
						t.Errorf("pending detail dispatched %s", request.Action)
					}
					data := state
					data.Entry, data.Password, data.Username, data.URL, data.Notes = request.Entry, "replacement-password", original.Username, original.URL, original.Notes
					return app.Response{Status: "ok", Data: data}
				})
				view := ui.NewTester(testView(a), size[0], size[1])
				view.Frame() // Bounds used by the placeholder come from the previous laid-out frame.
				labels := []string{"账号字段", "显示全部字段", "编辑条目", "移到回收站", "显示或隐藏密码", "复制密码", "复制用户名", "复制网址"}
				positions := make(map[string]ui.Rect)
				for _, label := range labels {
					position, found := view.Find(label)
					if !found {
						t.Fatalf("detail control missing before selection: %s", label)
					}
					positions[label] = position
				}
				a.selectEntry("工作/Figma")
				view.Frame()
				for _, label := range labels {
					position, found := view.Find(label)
					if !found || position != positions[label] {
						t.Fatalf("selection moved or removed %s: before=%v pending=%v", label, positions[label], position)
					}
				}
				if view.HasText(original.Password) {
					t.Fatal("pending detail revealed the previous password")
				}
				for _, value := range []string{original.Username, original.URL, original.Notes} {
					if !view.HasText(value) {
						t.Fatal("pending detail replaced the previous read-only display with a placeholder")
					}
				}
				for _, label := range []string{"显示全部字段", "编辑条目", "移到回收站", "显示或隐藏密码", "复制密码", "复制用户名", "打开网站"} {
					if err := view.Click(label); err != nil {
						t.Fatal(err)
					}
					if a.reveal || len(a.revealedFields) != 0 || a.editor.open || a.deleteOpen || a.errorText != "" || len(view.OpenedURLs()) != 0 {
						t.Fatalf("pending detail allowed a stale action: %s", label)
					}
				}
				awaitMain(t, callbacks)
				view.Frame()
				for _, label := range labels {
					position, found := view.Find(label)
					if !found || position != positions[label] {
						t.Fatalf("completed selection moved or removed %s: before=%v complete=%v", label, positions[label], position)
					}
				}
			})
		}
	}
}

func TestNewDetailSelectionReturnsToTop(t *testing.T) {
	a := fixture()
	a.detail.Notes = strings.Repeat("较长备注用于滚动详情。", 200)
	state := a.state
	callbacks := make(chan func(), 1)
	a.runOnMain = func(callback func()) { callbacks <- callback }
	a.service = backendFunc(func(request app.Request) app.Response {
		data := state
		data.Entry, data.Password = request.Entry, "new-password"
		return app.Response{Status: "ok", Data: data}
	})
	view := ui.NewTester(testView(a), 1120, 760)
	a.detailScroll.Y = a.detailScroll.MaxY
	view.Frame()
	if a.detailScroll.Y <= 0 {
		t.Fatal("fixture did not scroll a long detail")
	}
	previousOffset := a.detailScroll.Y
	a.selectEntry("工作/Figma")
	view.Frame()
	if a.detailScroll.Y != previousOffset {
		t.Fatal("a pending read jumped the previous presentation to the top")
	}
	awaitMain(t, callbacks)
	view.Frame()
	if a.detailScroll.Y != 0 {
		t.Fatalf("new selection inherited the previous entry scroll offset: %v", a.detailScroll.Y)
	}
}

func TestDelayedDetailFeedbackKeepsLongPathGeometry(t *testing.T) {
	for _, size := range [][2]int{{860, 620}, {1120, 760}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			a := fixture()
			a.busy, a.backgroundBusy = true, true
			a.detail.Entry = strings.Repeat("long-directory-name/", 7) + "Example"
			view := ui.NewTester(testView(a), size[0], size[1])
			// The target path wraps in both viewports. Showing delayed feedback
			// must not take width from that path and push the fields downward.
			a.selectEntry("工作/Figma")
			view.Frame()
			if view.HasText(a.t("正在读取账号…")) {
				t.Fatal("detail feedback appeared before its delay")
			}
			positions := make(map[string]ui.Rect)
			for _, label := range []string{"账号字段", "显示或隐藏密码", "复制密码", "复制用户名", "复制网址", "打开网站"} {
				position, found := view.Find(a.t(label))
				if !found {
					t.Fatalf("pending detail control missing: %s", label)
				}
				positions[label] = position
			}
			a.detailLoadingStarted = time.Now().Add(-detailLoadingDelay - time.Second)
			view.Frame()
			if !view.HasText(a.t("正在读取账号…")) {
				t.Fatal("slow detail did not show its delayed feedback")
			}
			for label, before := range positions {
				after, found := view.Find(a.t(label))
				if !found || after != before {
					t.Fatalf("delayed feedback moved or removed %s: before=%v after=%v", label, before, after)
				}
			}
		})
	}
}

func TestSearchFocusKeepsCurrentAndQueuedDetailReads(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run(fmt.Sprintf("queued=%t", queued), func(t *testing.T) {
			a := fixture()
			callbacks := make(chan func(), 2)
			requests := make(chan app.Request, 2)
			state := a.state
			a.runOnMain = func(callback func()) { callbacks <- callback }
			a.service = backendFunc(func(request app.Request) app.Response {
				requests <- request
				data := state
				data.Entry, data.Password = request.Entry, "synthetic-password"
				return app.Response{Status: "ok", Data: data}
			})
			a.selectEntry("工作/Figma")
			if queued {
				a.selectEntry("开发/Cloudflare")
			}
			serial, started, pending := a.detailSelectionSerial, a.detailLoadingStarted, a.pendingSelection
			view := ui.NewTester(testView(a), 1120, 760)
			view.Key(ui.Cmd, ui.KeyF)
			if a.detailSelectionSerial != serial || !a.detailLoadingStarted.Equal(started) || a.pendingSelection != pending || !a.detailTransitionPresentation(time.Now()).Pending {
				t.Fatal("focusing search cancelled or restarted the current detail read")
			}
			awaitMain(t, callbacks)
			if queued {
				awaitMain(t, callbacks)
			}
			if a.busy || a.detail.Entry != a.selected || a.detail.Password != "synthetic-password" || a.pendingSelection != "" {
				t.Fatal("focusing search left the selected detail permanently empty")
			}
			expected := 1
			if queued {
				expected++
			}
			if len(requests) != expected {
				t.Fatalf("focusing search dispatched an extra read: got %d, want %d", len(requests), expected)
			}
		})
	}
}

func TestCurrentEntryClickDuringBackgroundRefreshKeepsLoadedDetail(t *testing.T) {
	a := fixture()
	original := a.detail
	a.busy, a.backgroundBusy, a.activeRequestAction = true, true, "configure"
	a.selectEntry(a.selected)
	if !reflect.DeepEqual(a.detail, original) || a.pendingSelection != "" || a.detailTransitionPresentation(time.Now()).Pending {
		t.Fatal("clicking the selected account flashed its detail during a metadata read")
	}
}
