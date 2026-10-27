package discovery

import (
	"os"
	"path/filepath"
	"sort"
)

func Find(builtIn string, configured []string) []string {
	dirs := append([]string{}, configured...)
	if builtIn != "" {
		dirs = append(dirs, builtIn)
	}
	seen := map[string]bool{}
	seenIDs := map[string]bool{}
	var out []string
	for _, dir := range dirs {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(dir, e.Name(), "plugin.sh")
			if st, err := os.Stat(p); err == nil && st.Mode()&0111 != 0 {
				p, _ = filepath.Abs(p)
				id := e.Name()
				if !seen[p] && !seenIDs[id] {
					seen[p] = true
					seenIDs[id] = true
					out = append(out, p)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// BuiltinDirectories returns the development and installed locations relative
// to the executable. Installed binaries live in bin/ while plugins live in
// share/check-now/plugins.
func BuiltinDirectories(executable string) []string {
	dir := filepath.Dir(executable)
	return []string{
		filepath.Join(dir, "plugins"),
		filepath.Join(dir, "..", "share", "check-now", "plugins"),
	}
}

func ID(path string) string { return filepath.Base(filepath.Dir(path)) }
