package desktop

import (
	"fmt"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/knothhe/passpine/client/buildinfo"
)

func prepareDevelopmentUninstall() error {
	mygo.App.SetName(buildinfo.Name())
	directory, err := buildinfo.DataDir()
	if err != nil {
		return err
	}
	mygo.App.SetPath(mygo.PathUserData, directory)
	var result error
	mygo.App.WhenReady(func() {
		// MyGo 0.5.5 skips only NotRegistered. An ad-hoc signed Dev bundle
		// can instead report NotFound; unregistering that absent service
		// returns SMAppServiceErrorDomain/1 (Operation not permitted).
		class := objc.ID(objc.GetClass("SMAppService"))
		if class == 0 {
			result = mygo.App.SetOpenAtLogin(false) // macOS 12 fallback
		} else if service := class.Send(objc.RegisterName("mainAppService")); service == 0 {
			result = fmt.Errorf("cannot inspect the Dev application's login service")
		} else {
			status := objc.Send[int](service, objc.RegisterName("status"))
			result = removeDevelopmentLoginItem(status, func() error { return mygo.App.SetOpenAtLogin(false) })
		}
		mygo.App.Quit()
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return result
}

func removeDevelopmentLoginItem(status int, remove func() error) error {
	switch status {
	case 0, 3: // SMAppServiceStatusNotRegistered, SMAppServiceStatusNotFound
		return nil
	case 1, 2: // Enabled or RequiresApproval: both are registered services.
		return remove()
	default:
		return fmt.Errorf("unrecognized Dev login service status: %d", status)
	}
}
