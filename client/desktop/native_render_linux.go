package desktop

import "os"

func configureNativeRendering() {
	// MyGo 0.5.5 can present a second GTK surface without a current GL
	// context, which aborts in libepoxy instead of falling back. Keep Linux
	// windows on its Cairo software path until that lifecycle bug is fixed.
	// An explicit setting remains available for testing other renderers.
	if os.Getenv("MYGO_GPU") == "" {
		_ = os.Setenv("MYGO_GPU", "0")
	}
}
