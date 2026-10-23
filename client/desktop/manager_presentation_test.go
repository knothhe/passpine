package desktop

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

func TestEmptySearchRecoveryPreservesFolderAndReturnsFocus(t *testing.T) {
	a := fixture()
	a.filter, a.query = "folder:开发", "no-matching-entry"
	selected, detail := a.selected, a.detail
	view := ui.NewTester(testView(a), 1120, 760)
	if !view.HasText(a.t("没有找到条目")) || !view.HasText(a.t("清除搜索")) {
		t.Fatal("filtered empty state does not offer a clear recovery action")
	}
	if err := view.Click(a.t("清除搜索")); err != nil {
		t.Fatal(err)
	}
	if a.filter != "folder:开发" || a.query != "" || len(a.filtered) != 2 {
		t.Fatalf("clearing the keyword changed its folder or failed to restore its entries: filter=%q query=%q entries=%+v", a.filter, a.query, a.filtered)
	}
	if !view.Focused(a.t("搜索条目")) {
		t.Fatal("clearing search did not return keyboard focus to the search input")
	}
	if a.selected != selected || !reflect.DeepEqual(a.detail, detail) {
		t.Fatal("recovering a search discarded the current entry or its loaded detail")
	}
}

func TestListSubtitleUsesOnlyCurrentUnlockedDetail(t *testing.T) {
	a := fixture()
	github, figma := a.state.Entries[0], a.state.Entries[1]
	if got := a.entrySubtitle(github); got != a.detail.Username {
		t.Fatalf("the loaded entry does not expose its existing username: %q", got)
	}
	if got := a.entrySubtitle(figma); got != figma.Path {
		t.Fatalf("another entry borrowed the selected account's username: %q", got)
	}
	a.state.Locked = true
	if got := a.entrySubtitle(github); got != github.Path {
		t.Fatalf("a locked list retained a decrypted username: %q", got)
	}
	a.state.Locked = false
	a.detail = app.Data{}
	if got := a.entrySubtitle(github); got != github.Path {
		t.Fatalf("an unread entry did not use its existing path: %q", got)
	}
}

func TestDetailReadOnlyTransitionKeepsApprovedFieldOrder(t *testing.T) {
	a := fixture()
	a.detail.OTP = "123456"
	a.detail.Contents += "recovery: secret-recovery-value\n"
	fields := detailDisplayFields(a.detail)
	ordered := orderedDetailFields(fields)
	var keys []string
	for _, field := range ordered {
		keys = append(keys, field.Key)
		if field.Secret && field.Value != "" {
			t.Fatal("pending field ordering restored a secret value")
		}
	}
	want := []string{"username", "password", "otp", "url", "custom:0"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("the pending detail will jump back to a different field order: got %v, want %v", keys, want)
	}
}
