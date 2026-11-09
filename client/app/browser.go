package app

import (
	"errors"
	"time"

	"github.com/knothhe/passpine/client/i18n"
	"github.com/knothhe/passpine/client/nativehost"
)

type browserState struct {
	checkedAt          time.Time
	helperAvailable    bool
	registrations      []nativehost.BrowserRegistration
	configurationError string
}

func (s *Service) installBrowser(browserID string) (Data, error) {
	if s.preview {
		return Data{}, errors.New("演示模式不注册浏览器连接")
	}
	configurations, err := nativehost.LoadBrowserConfigurations(s.directory)
	if err != nil {
		return Data{}, err
	}
	_, err = nativehost.RegisterBrowserWithConfigurations(nativehost.HelperPath(), browserID, configurations)
	s.browser.checkedAt = time.Time{}
	return Data{}, err
}

func (s *Service) configureBrowser(r Request) (Data, error) {
	if s.preview {
		return Data{}, errors.New("演示模式不注册浏览器连接")
	}
	var err error
	if r.Action == "browser-remove" {
		err = nativehost.RemoveBrowserConfiguration(s.directory, nativehost.HelperPath(), r.ID)
	} else if r.BrowserConfiguration == nil {
		err = errors.New(i18n.BrowserConfigurationMissing)
	} else {
		err = nativehost.SaveBrowserConfiguration(s.directory, *r.BrowserConfiguration)
	}
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
		configurations, err := nativehost.LoadBrowserConfigurations(s.directory)
		s.browser.configurationError = ""
		if err != nil {
			s.browser.configurationError = err.Error()
		}
		s.browser.registrations = nativehost.RegistrationsWithConfigurations(helper, configurations)
		s.browser.checkedAt = time.Now()
	}
	data.BrowserHelperAvailable = s.browser.helperAvailable
	data.BrowserConfigurationError = s.browser.configurationError
	data.BrowserRegistrations = append([]nativehost.BrowserRegistration(nil), s.browser.registrations...)
	for _, browser := range data.BrowserRegistrations {
		data.BrowserInstalled = data.BrowserInstalled || browser.Registered
	}
}
