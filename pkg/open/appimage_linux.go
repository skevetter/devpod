package open

import (
	"os"
	"os/exec"
	"strings"
)

// isAppImage reports whether the current process is running inside an AppImage.
func isAppImage() bool {
	return os.Getenv("APPIMAGE") != ""
}

// openURLSanitized opens a URL using xdg-open with a sanitized environment
// to work around AppImage library conflicts.
//
// When running inside an AppImage, the AppRun script prepends the image's
// bundled library paths to LD_LIBRARY_PATH. Child processes like xdg-open
// and gio then load these bundled libraries instead of the system ones,
// causing symbol lookup errors (e.g. "undefined symbol: g_unix_mount_entry_get_options").
//
// This is a well-known AppImage limitation:
//   - https://github.com/AppImage/AppImageKit/issues/396
//   - https://github.com/AppImage/AppImageKit/issues/616
//
// Tauri's AppImage builds also bundle their own xdg-open wrapper
// (APPIMAGE_BUNDLE_XDG_OPEN=1), which compounds the issue:
//   - https://github.com/tauri-apps/tauri/issues/10617
//   - https://github.com/tauri-apps/plugins-workspace/pull/2103
//
// The fix is to strip AppImage-injected environment variables before spawning
// xdg-open, and to use the system xdg-open by absolute path to bypass any
// bundled wrapper.
func openURLSanitized(url string) error {
	// Use the system xdg-open to avoid Tauri's bundled wrapper.
	xdgOpen := "/usr/bin/xdg-open"
	if _, err := os.Stat(xdgOpen); err != nil {
		xdgOpen = "xdg-open"
	}

	//nolint:gosec // xdgOpen is either "/usr/bin/xdg-open" or "xdg-open"
	cmd := exec.Command(xdgOpen, url)
	cmd.Env = sanitizedEnv()
	return cmd.Run()
}

// sanitizedEnv returns a copy of the current environment with AppImage-injected
// variables removed or restored to their pre-AppImage values.
func sanitizedEnv() []string {
	// Variables injected by AppImage's AppRun that cause library conflicts
	// when inherited by system binaries.
	strip := map[string]bool{
		"APPDIR":                   true,
		"APPIMAGE":                 true,
		"ARGV0":                    true,
		"OWD":                      true,
		"APPIMAGE_BUNDLE_XDG_OPEN": true,
	}

	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strip[key] {
			continue
		}
		env = append(env, kv)
	}

	// Restore LD_LIBRARY_PATH to its pre-AppImage value if saved,
	// otherwise remove it entirely so system binaries use system libs.
	env = removeEnvKey(env, "LD_LIBRARY_PATH")
	if orig := os.Getenv("ORIG_LD_LIBRARY_PATH"); orig != "" {
		env = append(env, "LD_LIBRARY_PATH="+orig)
	}

	// LD_PRELOAD may be set to an exec interception library (exec.so);
	// remove it so system binaries aren't affected.
	env = removeEnvKey(env, "LD_PRELOAD")

	return env
}

func removeEnvKey(env []string, key string) []string {
	prefix := key + "="
	result := env[:0:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, prefix) {
			result = append(result, kv)
		}
	}
	return result
}
