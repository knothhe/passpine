package secureclipboard

import "github.com/egoist/mygo/transfer"

const (
	macTextType      = "public.utf8-plain-text"
	macConcealedType = "org.nspasteboard.ConcealedType"
	macTransientType = "org.nspasteboard.TransientType"

	windowsExcludeMonitor = "ExcludeClipboardContentFromMonitorProcessing"
	windowsAllowHistory   = "CanIncludeInClipboardHistory"
	windowsAllowCloud     = "CanUploadToCloudClipboard"
	linuxPasswordHint     = "x-kde-passwordManagerHint"
)

func windowsSensitiveData(value string) transfer.Data {
	return transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.Text, []byte(value)),
		transfer.Bytes(windowsExcludeMonitor, []byte{1}),
		// Windows expects four-byte little-endian DWORD values for these flags.
		transfer.Bytes(windowsAllowHistory, []byte{0, 0, 0, 0}),
		transfer.Bytes(windowsAllowCloud, []byte{0, 0, 0, 0}),
	))
}

func linuxSensitiveData(value string) transfer.Data {
	return transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.Text, []byte(value)),
		transfer.Bytes(linuxPasswordHint, []byte("secret")),
	))
}
