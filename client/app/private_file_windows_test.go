package app

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func assertPrivateFile(t *testing.T, path string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("private DACL permits inheritance", err)
	}
	_, aces, found := strings.Cut(sd.String(), "(")
	expected := "A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + user.User.Sid.String() + ")"
	if !found || aces != expected {
		t.Fatalf("unexpected private file ACL: %s", sd.String())
	}
}
