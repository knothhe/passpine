package app

import "sync"

// PauseForUpdate drains in-flight desktop, browser and Git requests before an
// explicitly confirmed restart. New requests cannot begin during verification.
// Failed installation preparation calls resume; successful preparation quits.
func (s *Service) PauseForUpdate() func() {
	s.cancelConfigSync()
	s.updateRequests.Lock()
	s.updatePaused.Store(true)
	s.updateRequests.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.updateRequests.Lock()
			s.updatePaused.Store(false)
			s.updateRequests.Unlock()
		})
	}
}
