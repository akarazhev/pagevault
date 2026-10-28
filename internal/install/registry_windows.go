//go:build windows

package install

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

// Invoke reg.exe with separate literal arguments; no shell parses Unicode,
// percent signs, ampersands, exclamation marks or spaces in user paths.
func updateRegistry(key, manifest string, uninstall bool) error {
	args := []string{"add", `HKCU\` + key, "/ve", "/t", "REG_SZ", "/d", manifest, "/f"}
	if uninstall {
		name, err := syscall.UTF16PtrFromString(key)
		if err != nil {
			return err
		}
		var handle syscall.Handle
		err = syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, name, 0, syscall.KEY_QUERY_VALUE, &handle)
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) || errors.Is(err, syscall.ERROR_PATH_NOT_FOUND) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("open native host registry key: %w", err)
		}
		syscall.RegCloseKey(handle)
		args = []string{"delete", `HKCU\` + key, "/f"}
	}
	output, err := exec.Command("reg.exe", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("update native host registry: %w: %s", err, output)
	}
	return nil
}
