//go:build !unix

package source

import "io/fs"

// owner reports that file ownership is not known on this platform.
func owner(fs.FileInfo) (uid, gid int, ok bool) { return 0, 0, false }
