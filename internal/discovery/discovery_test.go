package discovery

import (
	"path/filepath"
	"testing"
)

func TestInstalledPluginDirectoriesIncludeShareLocation(t *testing.T) {
	exe := "/home/test/.local/bin/check-now"
	dirs := BuiltinDirectories(exe)
	want := filepath.Clean("/home/test/.local/share/check-now/plugins")
	for _, got := range dirs {
		if got == want {
			return
		}
	}
	t.Fatalf("BuiltinDirectories(%q) did not include %q: %v", exe, want, dirs)
}
