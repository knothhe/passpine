package secureclipboard

import "github.com/egoist/mygo"

func writeSensitiveText(value string) error {
	return mygo.Clipboard.Write(linuxSensitiveData(value))
}
