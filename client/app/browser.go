package app

import (
	"errors"
	"time"

	"github.com/knothhe/passpine-neo/client/nativehost"
)

type browserState struct {
	checkedAt       time.Time
	helperAvailable bool
	registrations   []nativehost.BrowserRegistration
}

func (s *Service) installBrowser(browserID string) (Data, error) {
	if s.preview {
		return Data{}, errors.New("演示模式不注册浏览器连接")
	}
	_, err := nativehost.RegisterBrowser(nativehost.HelperPath(), browserID)
	s.browser.checkedAt = time.Time{}
	return Data{}, err
}

func (s *Service) browserSnapshot(data *Data) {
	if s.preview {
		return
	}
	// Status polling and credential requests should not repeatedly read every
	// browser manifest. A repair invalidates this short read-only cache.
	if time.Since(s.browser.checkedAt) >= 5*time.Second {
		helper := nativehost.HelperPath()
		s.browser.helperAvailable = nativehost.HelperAvailable(helper)
		s.browser.registrations = nativehost.Registrations(helper)
		s.browser.checkedAt = time.Now()
	}
	data.BrowserHelperAvailable = s.browser.helperAvailable
	data.BrowserRegistrations = append([]nativehost.BrowserRegistration(nil), s.browser.registrations...)
	for _, browser := range data.BrowserRegistrations {
		data.BrowserInstalled = data.BrowserInstalled || browser.Registered
	}
}
