//go:build unix

package source

import (
	"io/fs"
	"syscall"
)

// owner returns the user and group that own the file info describes.
func owner(info fs.FileInfo) (uid, gid int, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return int(st.Uid), int(st.Gid), true
}
