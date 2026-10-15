package app

import (
	"fmt"
	"path/filepath"
)

// DemoPassphrase only protects generated fictional preview fixtures.
const DemoPassphrase = "Passpine-demo-2026"

func (s *Service) createPreview() error {
	dir := filepath.Join(s.directory, "demo-vault")
	_, e := s.setup(Request{Action: "create", Directory: dir, Name: "个人密码库", Email: "demo@example.com", Passphrase: DemoPassphrase})
	if e != nil {
		return e
	}
	entries := []struct{ path, username, url, notes string }{
		{"工作/GitHub", "alex@example.com", "https://github.com", "工作代码仓库\n恢复代码请离线保管。"},
		{"工作/Linear", "alex@example.com", "https://linear.app", "团队项目与任务"},
		{"工作/Figma", "alex@studio.example", "https://figma.com", "设计协作账号"},
		{"工作/Vercel", "alex@example.com", "https://vercel.com", "项目部署账号"},
		{"个人/Apple", "alex@example.com", "https://apple.com", "个人设备与服务"},
		{"个人/Notion", "alex@example.com", "https://notion.so", "我的知识库"},
		{"个人/Proton Mail", "alex@proton.example", "https://proton.me", "个人邮箱"},
		{"个人/Spotify", "alex@example.com", "https://spotify.com", "音乐与播客"},
		{"生活/旅行", "alex@example.com", "https://travel.example", "旅行资料与行程"},
		{"服务器/开发环境", "deploy", "https://dev.example", "仅供演示的开发环境凭证"},
		{"服务器/家庭 NAS", "alex", "https://nas.example", "家庭文件存储"},
	}
	for i, item := range entries {
		raw := fmt.Sprintf("Demo-only-%02d!not-a-real-password\nusername: %s\nurl: %s\n\n%s\n", i, item.username, item.url, item.notes)
		if i == 0 {
			raw += "otpauth: otpauth://totp/Passpine:demo?secret=JBSWY3DPEHPK3PXP&issuer=Passpine\n"
		}
		if _, e = s.active.Save(item.path, raw, ""); e != nil {
			return e
		}
	}
	s.prefs.Favorites[s.prefs.Current] = map[string]bool{"工作/GitHub": true, "工作/Figma": true, "个人/Proton Mail": true}
	return s.savePreferences()
}
