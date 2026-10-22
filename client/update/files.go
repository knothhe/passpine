package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type codedError struct {
	code string
	err  error
}

func (e *codedError) Error() string      { return e.code + ": " + e.err.Error() }
func (e *codedError) Unwrap() error      { return e.err }
func coded(code string, err error) error { return &codedError{code, err} }
func pathJoin(parts ...string) string    { return filepath.Join(parts...) }

// canonicalPath resolves existing ancestors as well as an existing final path.
// Vault locations may be reached through a symlink outside the .app bundle.
func canonicalPath(name string) string {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return name
	}
	current := absolute
	var suffix []string
	for {
		resolved, e := filepath.EvalSymlinks(current)
		if e == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return absolute
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}
func protects(root, protected string) bool {
	absolute, err := filepath.Abs(protected)
	if err != nil {
		return true
	}
	return within(root, absolute) || within(canonicalPath(root), canonicalPath(absolute))
}
func within(root, child string) bool {
	r, e := filepath.Rel(root, child)
	return e == nil && (r == "." || r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)))
}
func secureDirectory(p string) error {
	if e := os.MkdirAll(p, 0o700); e != nil {
		return e
	}
	st, e := os.Lstat(p)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("update cache is not a real directory")
	}
	return os.Chmod(p, 0o700)
}
func writeAtomic(name string, data []byte, mode os.FileMode) error {
	f, e := os.CreateTemp(filepath.Dir(name), ".update-write-")
	if e != nil {
		return e
	}
	temp := f.Name()
	defer os.Remove(temp)
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	// Windows cannot rename over a pre-existing file. A short removal of cache
	// metadata never touches the installed application or its data.
	if e = os.Rename(temp, name); e != nil {
		if re := os.Remove(name); re != nil && !os.IsNotExist(re) {
			return e
		}
		e = os.Rename(temp, name)
	}
	return e
}
func regularFile(name string) (os.FileInfo, error) {
	st, e := os.Lstat(name)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("update file is not regular")
	}
	return st, nil
}
func securePath(root, relative string) (string, error) {
	if !safePath(relative) {
		return "", errors.New("unsafe update path")
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	if !within(root, target) {
		return "", errors.New("update path escapes root")
	}
	for p := filepath.Dir(target); within(root, p); p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		if e == nil {
			if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
				return "", errors.New("update parent is not a real directory")
			}
		} else if !os.IsNotExist(e) {
			return "", e
		}
		if p == root {
			break
		}
	}
	return target, nil
}
func verifyFile(name, want string, size int64) error {
	st, e := regularFile(name)
	if e != nil {
		return e
	}
	if st.Size() != size {
		return errors.New("update file size mismatch")
	}
	f, e := os.Open(name)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, size+1))
	if e != nil {
		return e
	}
	if n != size || hex.EncodeToString(h.Sum(nil)) != want {
		return errors.New("update checksum mismatch")
	}
	return nil
}
func verifyTree(root string, files []File) error {
	st, e := os.Lstat(root)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("update tree is not a real directory")
	}
	expected := map[string]bool{}
	for _, f := range files {
		name, e := securePath(root, f.Path)
		if e != nil {
			return e
		}
		if e = verifyFile(name, f.SHA256, f.Size); e != nil {
			return e
		}
		expected[filepath.Clean(name)] = true
	}
	return filepath.WalkDir(root, func(name string, entry os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if entry.IsDir() {
			return nil
		}
		if !expected[filepath.Clean(name)] {
			return errors.New("unexpected file in update tree")
		}
		return nil
	})
}
func copyFile(source, destination string, mode os.FileMode) error {
	if _, e := regularFile(source); e != nil {
		return e
	}
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	defer in.Close()
	if e = os.MkdirAll(filepath.Dir(destination), 0o755); e != nil {
		return e
	}
	out, e := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if e != nil {
		return e
	}
	e = out.Chmod(mode)
	if e == nil {
		_, e = io.Copy(out, io.LimitReader(in, maxFileSize+1))
	}
	if e == nil {
		e = out.Sync()
	}
	ce := out.Close()
	if e == nil {
		e = ce
	}
	return e
}
func (m *Manager) downloadAndStage(ctx context.Context, d Download, delta bool) error {
	m.publish(func(s *Snapshot) { s.State = StateDownloading; s.Progress = 0; s.Downloaded = 0; s.Total = d.Size })
	resp, e := m.request(ctx, d.URL)
	if e != nil {
		return coded(ErrorDownload, e)
	}
	defer resp.Body.Close()
	if resp.ContentLength >= 0 && resp.ContentLength != d.Size {
		return coded(ErrorVerification, errors.New("update content length mismatch"))
	}
	archive, e := os.CreateTemp(m.cacheDir(), "package-*.partial")
	if e != nil {
		return coded(ErrorStorage, e)
	}
	archiveName := archive.Name()
	defer os.Remove(archiveName)
	hash := sha256.New()
	var received int64
	buffer := make([]byte, 128<<10)
	for {
		n, re := resp.Body.Read(buffer)
		if n > 0 {
			received += int64(n)
			if received > d.Size {
				archive.Close()
				return coded(ErrorVerification, errors.New("update exceeds signed size"))
			}
			if _, e = archive.Write(buffer[:n]); e != nil {
				archive.Close()
				return coded(ErrorStorage, e)
			}
			hash.Write(buffer[:n])
			m.publish(func(s *Snapshot) { s.Downloaded = received; s.Progress = float64(received) / float64(d.Size) })
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			archive.Close()
			return coded(ErrorDownload, re)
		}
	}
	e = archive.Sync()
	ce := archive.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return coded(ErrorStorage, e)
	}
	if received != d.Size || hex.EncodeToString(hash.Sum(nil)) != d.SHA256 {
		return coded(ErrorVerification, errors.New("downloaded update checksum mismatch"))
	}
	m.publish(func(s *Snapshot) { s.State = StateVerifying; s.Progress = 0 })
	stage, e := os.MkdirTemp(m.cacheDir(), "stage-")
	if e != nil {
		return coded(ErrorStorage, e)
	}
	defer os.RemoveAll(stage)
	if e = m.extract(ctx, archiveName, stage, delta); e != nil {
		return coded(ErrorVerification, e)
	}
	if e = verifyTree(stage, m.asset.Files); e != nil {
		return coded(ErrorVerification, e)
	}
	ready := filepath.Join(m.cacheDir(), "ready")
	if e = os.RemoveAll(ready); e != nil {
		return coded(ErrorStorage, e)
	}
	if e = os.Rename(stage, ready); e != nil {
		return coded(ErrorStorage, e)
	}
	final := filepath.Join(m.cacheDir(), "package.zip")
	if e = os.Remove(final); e != nil && !os.IsNotExist(e) {
		return coded(ErrorStorage, e)
	}
	if e = os.Rename(archiveName, final); e != nil {
		return coded(ErrorStorage, e)
	}
	m.readyDir = ready
	return nil
}
func (m *Manager) extract(ctx context.Context, archive, stage string, delta bool) error {
	reader, e := zip.OpenReader(archive)
	if e != nil {
		return e
	}
	defer reader.Close()
	expected := make(map[string]File, len(m.asset.Files))
	for _, f := range m.asset.Files {
		expected[f.Path] = f
	}
	present := map[string]bool{}
	if len(reader.File) > len(expected)+8192 {
		return errors.New("too many archive entries")
	}
	for _, entry := range reader.File {
		if e = ctx.Err(); e != nil {
			return e
		}
		name := entry.Name
		if entry.FileInfo().IsDir() {
			if !safePath(strings.TrimSuffix(name, "/")) {
				return errors.New("unsafe archive directory")
			}
			continue
		}
		f, ok := expected[name]
		if !ok || present[name] || !entry.Mode().IsRegular() || entry.UncompressedSize64 != uint64(f.Size) {
			return fmt.Errorf("invalid archive member %q", name)
		}
		present[name] = true
		target, e := securePath(stage, name)
		if e != nil {
			return e
		}
		if e = os.MkdirAll(filepath.Dir(target), 0o755); e != nil {
			return e
		}
		in, e := entry.Open()
		if e != nil {
			return e
		}
		out, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(f.Mode))
		if e != nil {
			in.Close()
			return e
		}
		e = out.Chmod(os.FileMode(f.Mode))
		var n int64
		if e == nil {
			n, e = io.Copy(out, io.LimitReader(in, f.Size+1))
		}
		ie := in.Close()
		oe := out.Close()
		if e == nil {
			e = ie
		}
		if e == nil {
			e = oe
		}
		if e != nil {
			return e
		}
		if n != f.Size {
			return errors.New("archive member size mismatch")
		}
	}
	for i, f := range m.asset.Files {
		if !present[f.Path] {
			if !delta {
				return errors.New("full package omits signed file")
			}
			source, e := securePath(m.cfg.InstallRoot, f.Path)
			if e != nil {
				return e
			}
			if e = verifyFile(source, f.SHA256, f.Size); e != nil {
				return e
			}
			target, e := securePath(stage, f.Path)
			if e != nil {
				return e
			}
			if e = copyFile(source, target, os.FileMode(f.Mode)); e != nil {
				return e
			}
		}
		m.publish(func(s *Snapshot) { s.Progress = float64(i+1) / float64(len(m.asset.Files)) })
	}
	return nil
}
