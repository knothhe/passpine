package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func benchmarkVault(b *testing.B) *Vault {
	b.Helper()
	root := b.TempDir()
	v, e := New(Config{Directory: filepath.Join(root, "store"), KeyFile: filepath.Join(root, "key/secret.asc")})
	if e != nil {
		b.Fatal(e)
	}
	if _, e = v.Create("Benchmark", "benchmark@example.invalid", testPassphrase); e != nil {
		b.Fatal(e)
	}
	if _, e = v.Save("entry", "benchmark-password\nusername: benchmark\nurl: https://example.invalid", ""); e != nil {
		b.Fatal(e)
	}
	b.Cleanup(v.Lock)
	return v
}

func BenchmarkReadUnlocked(b *testing.B) {
	v := benchmarkVault(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e := v.Read("entry"); e != nil {
			b.Fatal(e)
		}
	}
}

func BenchmarkListMetadata1000(b *testing.B) {
	v := benchmarkVault(b)
	data, e := os.ReadFile(filepath.Join(v.config.Directory, "entry.gpg"))
	if e != nil {
		b.Fatal(e)
	}
	for i := 0; i < 999; i++ {
		if e = os.WriteFile(filepath.Join(v.config.Directory, fmt.Sprintf("item-%04d.gpg", i)), data, 0600); e != nil {
			b.Fatal(e)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entries, e := v.ListMetadata()
		if e != nil || len(entries) != 1000 {
			b.Fatal("list", e, len(entries))
		}
	}
}
