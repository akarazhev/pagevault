//go:build !windows

package install

import "errors"

func updateRegistry(key, manifest string, uninstall bool) error {
	return errors.New("Windows registry updates require Windows")
}
