package vault

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ImportSystemGPG selects only recipients named by this store's policies.
// Transferable keys stay in memory until ImportKeys saves a protected copy.
func (v *Vault) ImportSystemGPG(passphrase string) ([]Identity, error) {
	if passphrase == "" || strings.ContainsAny(passphrase, "\r\n\x00") {
		return nil, ErrPassphrase
	}
	token := v.generationToken()
	names, err := v.systemGPGRecipients()
	if err != nil {
		return nil, err
	}
	binary, err := findSystemGPG()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	public, err := runSystemGPG(ctx, binary, nil, append([]string{"--export", "--"}, names...)...)
	if err != nil {
		return nil, errors.New("无法读取系统 GPG 公钥，请检查 GPG 配置")
	}
	defer wipe(public)
	if len(public) == 0 {
		return nil, errors.New("系统 GPG 中没有与 .gpg-id 匹配的密钥")
	}
	keys, err := parseKeys(public)
	if err != nil {
		return nil, err
	}
	matched, err := resolveRecipients(names, keys)
	if err != nil {
		return nil, err
	}
	// Resolve names before requesting secrets, so ambiguous IDs cannot import
	// unrelated private keys. Public-only sharing recipients remain public.
	args := []string{"--pinentry-mode", "loopback", "--passphrase-fd", "0", "--export-secret-keys", "--"}
	for _, key := range matched {
		args = append(args, fingerprint(key))
	}
	input := []byte(passphrase + "\n")
	defer wipe(input)
	secret, err := runSystemGPG(ctx, binary, input, args...)
	if err != nil {
		return nil, errors.New("读取系统 GPG 私钥失败，请确认口令正确且匹配的私钥可导出；硬件卡私钥暂不支持")
	}
	defer wipe(secret)
	if len(secret) == 0 {
		return nil, errors.New("系统 GPG 中没有与 .gpg-id 匹配的可用私钥")
	}
	combined := make([]byte, 0, len(public)+len(secret))
	combined = append(combined, public...)
	combined = append(combined, secret...)
	defer wipe(combined)
	return v.importKeys(combined, passphrase, token)
}

func (v *Vault) systemGPGRecipients() ([]string, error) {
	v.operations.Lock()
	defer v.operations.Unlock()
	names, err := v.recipients("")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	add := func(names []string) error {
		for _, name := range names {
			if len(name) > 1024 || strings.ContainsRune(name, '\x00') || len(seen) >= 1024 {
				return errors.New(".gpg-id 收件人配置过大或包含无效字符")
			}
			seen[name] = true
		}
		return nil
	}
	if err = add(names); err != nil {
		return nil, err
	}
	err = filepath.WalkDir(v.config.Directory, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == v.config.Directory {
			return nil
		}
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != ".gpg-id" {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrPath
		}
		relative, err := filepath.Rel(v.config.Directory, filename)
		if err != nil {
			return err
		}
		names, err := v.recipients(path.Join(path.Dir(filepath.ToSlash(relative)), "entry"))
		if err != nil {
			return err
		}
		return add(names)
	})
	if err != nil {
		return nil, err
	}
	names = nil
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func findSystemGPG() (string, error) {
	if binary, err := exec.LookPath("gpg"); err == nil {
		return binary, nil
	}
	// Finder-launched macOS apps do not inherit a shell's Homebrew PATH.
	for _, binary := range []string{"/opt/homebrew/bin/gpg", "/usr/local/bin/gpg", "/usr/bin/gpg"} {
		if filename, err := exec.LookPath(binary); err == nil {
			return filename, nil
		}
	}
	return "", errors.New("未找到系统 GPG，请安装 GnuPG 后重试")
}

type gpgOutput struct{ buffer bytes.Buffer }

func (b *gpgOutput) Write(data []byte) (int, error) {
	if len(data) > maxFileSize-b.buffer.Len() {
		return 0, errors.New("GPG key export exceeds size limit")
	}
	return b.buffer.Write(data)
}

func runSystemGPG(ctx context.Context, binary string, input []byte, args ...string) ([]byte, error) {
	base := []string{"--no-options", "--batch", "--yes", "--no-tty", "--no-auto-key-retrieve", "--output", "-"}
	cmd := exec.CommandContext(ctx, binary, append(base, args...)...)
	cmd.WaitDelay = time.Second
	cmd.Stdin = bytes.NewReader(input)
	var output gpgOutput
	cmd.Stdout = &output
	// GPG diagnostics can contain private paths and user IDs. Surface a fixed
	// actionable error instead of persisting them in logs or UI responses.
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		wipe(output.buffer.Bytes())
		return nil, err
	}
	return output.buffer.Bytes(), nil
}
