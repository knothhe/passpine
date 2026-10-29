package desktop

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
)

// The website shows the real desktop surface in each supported language.
// scripts/capture-website-screenshots.mjs sets the output directory; ordinary
// test runs only check that both captures render.
func TestWebsiteScreenshots(t *testing.T) {
	directory := os.Getenv("PASSPINE_WEBSITE_SCREENSHOT_DIR")
	for _, language := range []string{"zh-CN", "en"} {
		t.Run(language, func(t *testing.T) {
			a := websiteScreenshotFixture(language)
			view := ui.NewTester(testView(a), 1120, 760)
			view.SetPreferences(ui.Preferences{ReduceMotion: true})
			view.SetScale(2)
			view.SetDark(false)
			view.Advance(time.Second)
			view.Frame()
			image := view.Image()
			if bounds := image.Bounds(); bounds.Dx() != 2240 || bounds.Dy() != 1520 {
				t.Fatalf("website screenshot is not a 2x 1120×760 capture: %v", bounds)
			}
			if !view.HasText(a.detail.Username) || view.HasText(a.detail.Password) {
				t.Fatal("website screenshot must show the demo account without revealing its password")
			}
			if directory == "" {
				return
			}
			path := filepath.Join(directory, language, "desktop.png")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := png.Encode(file, image); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Every value is fictional demo data, written in the screenshot's language.
func websiteScreenshotFixture(language string) *application {
	vault, work, development, personal := "个人密码库", "工作", "开发", "生活"
	notes := "工作与开源项目使用的账户。"
	if language == "en" {
		vault, work, development, personal = "Personal vault", "Work", "Development", "Personal"
		notes = "Account for work and open-source projects."
	}
	a := newApplication(nil)
	a.state = app.Data{Configured: true, VaultName: vault, Preview: true, Settings: app.DefaultSettings(), Entries: []app.Entry{
		{Path: development + "/GitHub", Name: "GitHub", Folder: development},
		{Path: work + "/Figma", Name: "Figma", Folder: work},
		{Path: personal + "/Apple", Name: "Apple", Folder: personal},
		{Path: development + "/Cloudflare", Name: "Cloudflare", Folder: development},
		{Path: work + "/Linear", Name: "Linear", Folder: work},
	}}
	a.state.Settings.Language = language
	a.state.Settings.Theme = "light"
	a.state.GitAvailable, a.state.GitInitialized = true, true
	a.state.GitBranch, a.state.GitRemote, a.state.GitUpstream = "main", "git@example.test:pine/store.git", "origin/main"
	a.state.GitStatusAt = "2026-10-29T08:00:00Z"
	a.selected = development + "/GitHub"
	a.detail = app.Data{Entry: a.selected, Password: "fake-Secret-42!", Username: "pine@example.test", URL: "https://github.com", Notes: notes,
		Contents: "fake-Secret-42!\nusername: pine@example.test\nurl: https://github.com\nnotes: " + notes + "\n", Revision: "website-screenshot"}
	return a
}
