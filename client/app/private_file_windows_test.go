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
	if matches, err := privateFileACLMatches(sd, user.User.Sid); err != nil || !matches {
		t.Fatalf("unexpected private file ACL: %s: %v", sd.String(), err)
	}
}

func privateFileACLMatches(sd *windows.SECURITY_DESCRIPTOR, user *windows.SID) (bool, error) {
	control, _, err := sd.Control()
	if err != nil {
		return false, err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return false, nil
	}
	// Windows can format a numerical account SID as an SDDL alias, such as
	// LA for the built-in Administrator. Normalize the expected descriptor
	// through the same Windows API, while still requiring exactly these ACEs.
	expected, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + user.String() + ")")
	if err != nil {
		return false, err
	}
	_, aces, found := strings.Cut(sd.String(), "(")
	_, expectedACEs, _ := strings.Cut(expected.String(), "(")
	return found && aces == expectedACEs, nil
}

func TestPrivateFileACLMatchesAccountAliasesAndRejectsChangedPermissions(t *testing.T) {
	// The numerical Local System SID canonicalizes to SY on every Windows
	// machine, exercising the same numerical-SID/alias mismatch as RID 500.
	user, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, descriptor string
		want             bool
	}{
		{"account-alias", "D:PAI(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;SY)", true},
		{"numerical-account", "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;S-1-5-18)", true},
		{"missing-current-user", "D:P(A;;FA;;;SY)(A;;FA;;;BA)", false},
		{"different-account", "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;AU)", false},
		{"extra-everyone-grant", "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;SY)(A;;FR;;;WD)", false},
		{"changed-access", "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FR;;;SY)", false},
		{"inherited-account", "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;ID;FA;;;SY)", false},
		{"unprotected", "D:(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;SY)", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString(test.descriptor)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := privateFileACLMatches(sd, user); err != nil || got != test.want {
				t.Fatalf("private ACL match = %t, want %t: %v", got, test.want, err)
			}
		})
	}
}
