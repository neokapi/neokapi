//go:build windows

package dataroot

import (
	"fmt"
	"syscall"
)

// identity is the volume serial number and file index of the directory at
// path: the same for every spelling of it and for it renamed on its volume,
// different for another directory. NTFS keeps a file index for the life of
// the file and changes its sequence part when the index is reused.
func identity(path string) (string, error) {
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

// followable reports whether a rename may be followed by identity. NTFS
// changes the sequence part of a file index when the index is reused.
func followable(string) bool { return true }
