//go:build !windows

package fileutil

import "os"

func RenameReplace(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
