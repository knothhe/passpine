package app

import (
	"crypto/rand"
	"errors"
	"time"

	"github.com/knothhe/passpine/client/vault"
)

const folderUndoLifetime = 5 * time.Minute
const maxFolderUndoReceipts = 128

type folderUndoReceipt struct {
	vaultID string
	vault   *vault.Vault
	folders []string
	expires time.Time
}

// Receipts are process-local capabilities. Their opaque identifiers never
// encode folder names, and reopening even the same vault invalidates them.
func (s *Service) pruneFolderUndos(now time.Time) {
	for id, receipt := range s.folderUndos {
		if receipt.vault != s.active || receipt.vaultID != s.prefs.Current || !now.Before(receipt.expires) {
			delete(s.folderUndos, id)
		}
	}
}

func (s *Service) cleanupFolders() (Data, error) {
	now := time.Now()
	s.pruneFolderUndos(now)
	removed, err := s.active.CleanupEmptyFoldersWithPaths()
	data := Data{RemovedFolders: len(removed)}
	if len(removed) > 0 {
		if s.folderUndos == nil {
			s.folderUndos = make(map[string]folderUndoReceipt)
		}
		if len(s.folderUndos) >= maxFolderUndoReceipts {
			var oldest string
			for id, receipt := range s.folderUndos {
				if oldest == "" || receipt.expires.Before(s.folderUndos[oldest].expires) {
					oldest = id
				}
			}
			delete(s.folderUndos, oldest)
		}
		data.UndoID = rand.Text()
		s.folderUndos[data.UndoID] = folderUndoReceipt{s.prefs.Current, s.active, removed, time.Now().Add(folderUndoLifetime)}
	}
	return data, folderOperationError(err, "无法清理空文件夹，请检查目录权限后重试。")
}

func (s *Service) undoCleanupFolders(request Request) (Data, error) {
	s.pruneFolderUndos(time.Now())
	receipt, ok := s.folderUndos[request.ID]
	if !ok || request.VaultID == "" || request.VaultID != receipt.vaultID {
		return Data{}, errors.New("撤销已失效，请重新操作。")
	}
	restored, remaining, err := s.active.RestoreEmptyFolders(receipt.folders)
	data := Data{RestoredFolders: restored, RemainingFolders: len(remaining)}
	if len(remaining) == 0 {
		delete(s.folderUndos, request.ID)
	} else {
		receipt.folders = remaining
		s.folderUndos[request.ID] = receipt
		data.UndoID = request.ID
	}
	return data, folderOperationError(err, "无法撤销空文件夹清理，请检查目录权限后重试。")
}

func folderOperationError(err error, message string) error {
	if err == nil {
		return nil
	}
	// Filesystem errors can contain private paths. Only metadata-free errors
	// cross the app boundary, including while locked browsing is disabled.
	if errors.Is(err, vault.ErrBusy) {
		return vault.ErrBusy
	}
	if errors.Is(err, vault.ErrPath) {
		return vault.ErrPath
	}
	return errors.New(message)
}
