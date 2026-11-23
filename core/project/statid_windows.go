//go:build windows

package project

import (
	"fmt"
	"os"
	"syscall"
)

// dirIdentity is the volume serial number and file index of the directory at
// path: the same for every spelling of one directory and for a directory
// renamed on its volume, different for a copy.
func dirIdentity(path string) (string, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := syscall.CreateFile(p, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil,
		syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer func() { _ = syscall.CloseHandle(h) }() // a read-only handle: nothing to flush
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}

// keyPrivate refuses a key that is not a regular file in a directory. On
// Windows the key stays private through the access control of the user's
// profile directory, which mode bits do not describe.
func keyPrivate(_, path string, dinfo, info os.FileInfo) error {
	if !dinfo.IsDir() || !info.Mode().IsRegular() {
		return fmt.Errorf("project: the state key %s must be a regular file in a directory; remove it and vouch for each checkout's state again", path)
	}
	return nil
}
