package app

import (
	"encoding/json"
	"path/filepath"
	"time"
)

// pruneActivity applies both retention limits, including when recording is off.
// The service mutex must be held once the service has started.
func (s *Service) pruneActivity(now time.Time) error {
	cutoff := now.Add(-time.Duration(s.prefs.Settings.ActivityRetentionDays) * 24 * time.Hour)
	retained := make([]Activity, 0, min(len(s.activity), s.prefs.Settings.ActivityMaxEntries))
	for _, item := range s.activity {
		at, err := time.Parse(time.RFC3339, item.Time)
		if err != nil || !at.After(cutoff) {
			continue
		}
		if len(retained) < s.prefs.Settings.ActivityMaxEntries {
			retained = append(retained, item)
		}
	}
	s.activityPrunedAt = now
	if len(retained) == len(s.activity) {
		return nil
	}
	previous := s.activity
	s.activity = retained
	if err := s.saveActivity(); err != nil {
		s.activity = previous
		return err
	}
	return nil
}

func (s *Service) saveActivity() error {
	b, err := json.Marshal(s.activity)
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(s.directory, "activity.json"), b, false)
}
