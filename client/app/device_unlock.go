package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

func (s *Service) deviceFile() string {
	if s.active == nil {
		return ""
	}
	return filepath.Join(filepath.Dir(s.active.Config().KeyFile), "device-unlock.json")
}
func (s *Service) deviceEnrolled() bool {
	if s.active == nil || !s.prefs.Settings.DeviceUnlock || s.preview {
		return false
	}
	_, err := readPrivate(s.deviceFile(), 1<<20)
	return err == nil
}
func (s *Service) cancelDeviceAuthentication() {
	s.deviceMu.Lock()
	defer s.deviceMu.Unlock()
	if s.deviceCancel != nil {
		s.deviceCancel()
	}
}
func (s *Service) deviceContext(epoch uint64) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	s.deviceMu.Lock()
	s.deviceCancel = cancel
	if epoch != s.cancellation.Load() {
		cancel()
	}
	s.deviceMu.Unlock()
	return ctx, func() { cancel(); s.deviceMu.Lock(); s.deviceCancel = nil; s.deviceMu.Unlock() }
}
func (s *Service) enrollDevice(passphrase string, epoch uint64) string {
	if s.preview || s.device == nil || !s.device.Available() || !s.prefs.Settings.DeviceUnlock || s.deviceEnrolled() || passphrase == "" || s.active == nil {
		return ""
	}
	generation, err := s.active.SessionGeneration()
	if err != nil {
		return ""
	}
	ctx, cancel := s.deviceContext(epoch)
	defer cancel()
	secret := []byte(passphrase)
	defer clear(secret)
	file := s.deviceFile()
	envelope, err := s.device.Enroll(ctx, filepath.Dir(file), secret)
	defer clear(envelope)
	if err == nil {
		err = s.active.WithSession(generation, func() error { return writePrivate(file, envelope, false) })
	}
	if err != nil {
		return "系统解锁未启用，可继续使用私钥口令"
	}
	return "已启用 Touch ID 或 Mac 登录密码解锁"
}
func (s *Service) unlockDevice(epoch uint64) error {
	if s.preview || s.device == nil || !s.device.Available() || !s.deviceEnrolled() {
		return errors.New("系统解锁尚未就绪，请先使用私钥口令解锁")
	}
	file := s.deviceFile()
	envelope, err := readPrivate(file, 1<<20)
	if err != nil {
		return err
	}
	defer clear(envelope)
	ctx, cancel := s.deviceContext(epoch)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.active.UnlockWithProvider(func() ([]byte, error) { return s.device.Unlock(ctx, filepath.Dir(file), envelope) })
}

type browserUnlockError struct {
	code    string
	message string
}

func (e *browserUnlockError) Error() string { return e.message }

// Only a user's explicit browser action reaches system authentication. Passive
// status/list requests never open a prompt or activate the manager window.
// The caller holds s.mu, while Lock/Close can still cancel authentication using
// deviceMu and the vault's independent generation lock.
func (s *Service) unlockForBrowser(epoch uint64) error {
	if err := s.requireVault(); err != nil {
		if s.onOpen != nil {
			go s.onOpen()
		}
		return err
	}
	if s.active.Status().Unlocked {
		return nil
	}
	if s.preview || s.device == nil || !s.device.Available() || !s.deviceEnrolled() {
		if s.onOpen != nil {
			go s.onOpen()
		}
		return &browserUnlockError{code: "system_unlock_required", message: "请先在客户端使用私钥口令解锁，并启用 Touch ID 或 Mac 登录密码解锁，然后重试此操作"}
	}
	if err := s.unlockDevice(epoch); err != nil {
		return &browserUnlockError{code: "authentication_cancelled", message: "系统验证未完成，请重试或在客户端使用私钥口令解锁"}
	}
	s.cachedAt = time.Time{}
	s.entriesVersion++
	if s.onChange != nil {
		callback := s.onChange
		go callback()
	}
	return nil
}
func (s *Service) removeDeviceEnvelopes() error {
	for _, v := range s.prefs.Vaults {
		file := filepath.Join(s.directory, "keys", idFor(v.Directory), "device-unlock.json")
		if _, err := readPrivate(file, 1<<20); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := os.Remove(file); err != nil {
			return err
		}
	}
	return nil
}
