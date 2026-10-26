package desktop

import (
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/passpine/client/app"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// The subtitle may use only the credential already read by the inspector.
// Listing and searching entries must never trigger additional decryptions, and
// locking immediately falls back to the non-secret entry path.
func (a *application) entrySubtitle(entry app.Entry) string {
	if !a.state.Locked && a.detail.Entry == entry.Path && a.detail.Username != "" {
		return a.detail.Username
	}
	return entry.Path
}

func detailFolder(path string) string {
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[:index]
	}
	return ""
}

func detailDomain(value string) string {
	parsed, err := url.Parse(value)
	if err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return value
}

func (a *application) detailBreadcrumb(c *ui.Context, path string, pending bool) {
	folder := detailFolder(path)
	label := folder
	if label == "" {
		label = a.t("全部条目")
	}
	ui.Row(c).Key("detail-path").Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(6).Label(a.t("条目路径")).Children(func() {
		parent := ui.ButtonBase(c).Key("detail-parent").MaxWidth(180).MinWidth(0).Shrink(1).Padding(4, 0).Gap(6).AlignItems(ui.Center).Tooltip(path)
		parent.Children(func() {
			components.Icon(c, lucide.Folder()).Size(12, 12).Shrink(0).TextColor(c.Theme().TextMuted)
			components.Text(c, label).SingleLine().Ellipsis("…").MinWidth(0).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
		})
		if parent.Clicked() && !pending {
			filter := "all"
			if folder != "" {
				filter = "folder:" + folder
			}
			a.navigateManager(filter)
		}
		components.Icon(c, lucide.ChevronRight()).Size(12, 12).Shrink(0).TextColor(c.Theme().TextMuted)
		components.Text(c, entryName(path)).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1).FontSize(scaledFontSize(c, 11)).TextColor(c.Theme().TextMuted)
	})
}

func (a *application) detailHero(c *ui.Context, path, website string) {
	ui.Column(c).Key("account-heading").FillWidth().MinWidth(0).Margin(20, 0, 24, 0).Gap(4).Children(func() {
		components.Text(c, entryName(path)).FillWidth().FontSize(scaledFontSize(c, 24)).Bold().Wrap()
		if website != "" {
			components.Text(c, detailDomain(website)).FillWidth().FontSize(scaledFontSize(c, 12)).TextColor(c.Theme().TextMuted).Wrap()
		}
	})
}

func (a *application) detailMetadata(c *ui.Context, path string) {
	ui.Row(c).Key("detail-metadata").FillWidth().MinWidth(0).Margin(24, 0, 0, 0).Padding(12, 0, 0, 0).Gap(8).AlignItems(ui.Start).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
		components.Text(c, a.t("加密文件")).Shrink(0).FontSize(scaledFontSize(c, 10)).TextColor(c.Theme().TextMuted)
		// Entry paths are pass-relative and omit the encrypted file extension.
		components.Text(c, path+".gpg").MinWidth(0).Grow(1).Wrap().Font("monospace").FontSize(scaledFontSize(c, 10)).TextColor(c.Theme().TextMuted)
	})
}

// Keep the old read-only surface in the same order while a replacement entry
// is read. The transition snapshot continues to exclude secret field values.
func orderedDetailFields(fields []detailDisplayField) []detailDisplayField {
	ordered := make([]detailDisplayField, 0, len(fields))
	for _, key := range []string{"username", "password", "otp", "url"} {
		for _, field := range fields {
			if field.Key == key {
				ordered = append(ordered, field)
			}
		}
	}
	for _, field := range fields {
		if strings.HasPrefix(field.Key, "custom:") {
			ordered = append(ordered, field)
		}
	}
	return ordered
}
