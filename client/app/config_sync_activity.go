package app

import (
	"errors"

	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/knothhe/r2connect"
)

// Record only recognized operations and controlled diagnostics. Requests,
// configuration, provider response bodies and error text never enter activity.
// The service mutex must be held once the service has started.
func (s *Service) logConfigSync(action, source string, err error) {
	switch action {
	case "config-sync-save", "config-sync-test", "config-sync-now", "config-sync-resolve",
		"config-sync-pause", "config-sync-resume", "config-sync-oauth-start",
		"config-sync-oauth-buckets", "config-sync-oauth-accounts", "config-sync-oauth-reload",
		"config-sync-cancel", "config-sync-disconnect":
	default:
		return
	}
	item := Activity{Action: action, Source: source, Outcome: "ok"}
	if err != nil {
		item.Outcome = "error"
		item.ErrorCode = syncErrorCode(err)
		var objectError *cloudsync.Error
		var apiError *r2connect.APIError
		if errors.As(err, &objectError) {
			item.HTTPStatus = objectError.Status
		} else if errors.As(err, &apiError) {
			item.HTTPStatus = apiError.StatusCode
		}
	}
	s.recordActivity(item)
}
