package desktop

import (
	"errors"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/knothhe/passpine/client/cloudsync"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

type configurationSyncTransfer struct {
	writing    bool
	current    func() bool
	options    app.ConfigSyncOptions
	manual, r2 configurationSyncDraft
	mode       int
	editing    bool
}

func (a *application) configSyncTransferView(c *ui.Context) {
	ui.Column(c).Key("sync-config-transfer").FillWidth().MinWidth(0).Padding(20).Gap(10).Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(10).Children(func() {
		a.configSyncSectionTitle(c, a.t("导入与导出"), lucide.File)
		muted(c, a.t("通过 JSON 文件迁移同步选项和连接参数，不包含访问密钥、授权或加密口令。"))
		ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
			if components.Button(c, a.t("导入配置…"), components.ButtonOutline).Disabled(a.configSyncSaveDisabled()).Clicked() {
				a.importConfigSync()
			}
			if components.Button(c, a.t("导出配置…"), components.ButtonOutline).Disabled(a.configSyncSaveDisabled()).Clicked() {
				a.exportConfigSync()
			}
			if a.configSync.transferWriting() {
				muted(c, a.t("正在导出同步配置…"))
			}
		})
	})
}

func (state *configurationSyncUI) transferWriting() bool {
	return state.transfer != nil && state.transfer.writing
}

// File panels are cancellable without blocking shutdown. Once a file write
// starts it owns the selected destination and the normal quit drain waits for it.
func (a *application) cancelConfigSyncTransfer() {
	if !a.configSync.transferWriting() {
		a.configSync.transfer = nil
	}
}

func (a *application) beginConfigSyncTransfer() *configurationSyncTransfer {
	if a.configSyncSaveDisabled() {
		return nil
	}
	a.beginOperationFeedback()
	lease := a.preferencesLease()
	transfer := &configurationSyncTransfer{
		current: func() bool { return lease() && !a.quit.requested && !a.updates.preparing && !a.state.Preview },
		options: a.configSync.options, manual: a.configSync.manual, r2: a.configSync.r2,
		mode: a.configSync.mode, editing: a.configSync.editing,
	}
	a.configSync.transfer = transfer
	a.invalidate()
	return transfer
}

func (a *application) configSyncTransferCurrent(transfer *configurationSyncTransfer) bool {
	return a.configSync.transfer == transfer && transfer.current()
}

func (a *application) finishConfigSyncTransfer(transfer *configurationSyncTransfer) {
	if a.configSync.transfer == transfer {
		a.configSync.transfer = nil
	}
	a.invalidate()
	a.continueQuit()
}

func (a *application) importConfigSync() {
	transfer := a.beginConfigSyncTransfer()
	if transfer == nil {
		return
	}
	options := mygo.OpenDialogOptions{Parent: a.dialogParent(), Title: a.t("导入同步配置"), Filters: []mygo.FileFilter{{Name: a.t("Passpine 同步配置"), Extensions: []string{"json"}}}}
	open := a.preferences.openDialog
	if open == nil {
		open = mygo.Dialog.Open
	}
	read := a.configSync.readTransfer
	if read == nil {
		read = app.ReadConfigSyncTransferFile
	}
	go func() {
		paths, err := open(options)
		a.runOnMain(func() {
			if !a.configSyncTransferCurrent(transfer) {
				a.finishConfigSyncTransfer(transfer)
				return
			}
			if err != nil || len(paths) == 0 || paths[0] == "" {
				if err != nil {
					a.errorText = a.t("无法打开文件选择器，请重试。")
				}
				a.finishConfigSyncTransfer(transfer)
				return
			}
			go func() {
				imported, connection, readErr := read(paths[0])
				a.runOnMain(func() {
					defer a.finishConfigSyncTransfer(transfer)
					if !a.configSyncTransferCurrent(transfer) {
						return
					}
					if readErr != nil {
						if errors.Is(readErr, app.ErrInvalidConfigSyncTransfer) {
							a.errorText = a.t("同步配置文件无效、版本不支持或超过 64 KiB，请选择有效的导出文件。")
						} else {
							a.errorText = a.t("无法读取同步配置文件，请检查文件后重试。")
						}
						return
					}
					if a.configSync.options != transfer.options || a.configSync.manual != transfer.manual || a.configSync.r2 != transfer.r2 || a.configSync.mode != transfer.mode || a.configSync.editing != transfer.editing {
						a.errorText = a.t("导入期间配置已更改，当前编辑已保留，请重新导入。")
						return
					}
					a.configSync.options, a.configSync.optionsErrorCode = imported, ""
					if connection != nil {
						a.configSync.manual = configurationSyncDraft{config: *connection}
						a.configSync.mode, a.configSync.editing, a.configSync.disconnectConfirm = 1, true, false
						a.inform(a.t("配置已导入草稿。请保存同步选项，并补齐凭据后保存连接。"))
					} else {
						a.inform(a.t("同步选项已导入草稿，保存后生效。"))
					}
				})
			}()
		})
	}()
}

func (a *application) exportConfigSync() {
	if a.configSyncSaveDisabled() {
		return
	}
	connection, err := a.configSyncExportConnection()
	if err != nil {
		a.errorText = a.t("当前连接参数不完整，请补全后再导出。")
		a.invalidate()
		return
	}
	encoded, err := app.EncodeConfigSyncTransfer(a.configSync.options, connection)
	if err != nil {
		a.errorText = a.t("当前同步配置无效，请检查选项和连接参数后重试。")
		a.invalidate()
		return
	}
	transfer := a.beginConfigSyncTransfer()
	if transfer == nil {
		return
	}
	options := mygo.SaveDialogOptions{Parent: a.dialogParent(), Title: a.t("导出同步配置"), DefaultPath: "passpine-sync-config.json", Filters: []mygo.FileFilter{{Name: a.t("Passpine 同步配置"), Extensions: []string{"json"}}}, CreateDirectories: true}
	save := a.preferences.saveDialog
	if save == nil {
		save = mygo.Dialog.Save
	}
	write := a.configSync.writeTransfer
	if write == nil {
		write = app.WriteConfigSyncTransferFile
	}
	go func() {
		path, err := save(options)
		a.runOnMain(func() {
			if !a.configSyncTransferCurrent(transfer) {
				a.finishConfigSyncTransfer(transfer)
				return
			}
			if err != nil || path == "" {
				if err != nil {
					a.errorText = a.t("无法打开文件选择器，请重试。")
				}
				a.finishConfigSyncTransfer(transfer)
				return
			}
			transfer.writing = true
			a.invalidate()
			go func() {
				writeErr := write(path, encoded)
				a.runOnMain(func() {
					defer a.finishConfigSyncTransfer(transfer)
					if writeErr != nil && a.quit.requested {
						a.cancelQuitOnSaveFailure()
						a.errorText = a.configSyncExportError(writeErr)
						if a.lifecycle.nativeReady {
							a.show()
						}
						return
					}
					if !transfer.current() {
						return
					}
					if writeErr != nil {
						a.errorText = a.configSyncExportError(writeErr)
					} else {
						a.inform(a.t("同步配置已导出。"))
					}
				})
			}()
		})
	}()
}

func (a *application) configSyncExportError(err error) string {
	if errors.Is(err, os.ErrExist) {
		return a.t("目标文件已存在，请更换文件名后重试。")
	}
	return a.t("无法导出同步配置，请检查保存位置后重试。")
}

func (a *application) configSyncExportConnection() (*cloudsync.Config, error) {
	config := a.configSync.status.Config
	if a.configSync.editing || a.configSync.status.Provider == "" {
		config = a.configSync.manual.config
		if a.configSync.mode == 0 {
			// Bucket-selection drafts do not have a prepared endpoint yet. A
			// same-named bucket on another account is still a different target.
			if a.configSync.status.OAuthStage == "bucket" {
				return nil, app.ErrInvalidConfigSyncTransfer
			}
			config = a.configSync.r2.config
			if a.configSync.r2.bucket != "" && a.configSync.r2.bucket != config.Bucket {
				return nil, app.ErrInvalidConfigSyncTransfer
			}
		}
	}
	if config.Endpoint == "" && config.Bucket == "" {
		return nil, nil
	}
	normalized, err := cloudsync.NormalizeConfig(config)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}
