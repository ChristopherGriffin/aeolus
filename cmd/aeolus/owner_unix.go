//go:build unix

package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// checkOwner refuses to touch the change log as anyone but the user who owns
// its directory. A file SQLite creates as root (its -wal or -shm) would lock
// the service, which runs as that user, out of its own database.
func checkOwner(db string) error {
	st, err := os.Stat(filepath.Dir(db))
	if err != nil {
		return err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok || int(sys.Uid) == os.Geteuid() {
		return nil
	}
	name := strconv.Itoa(int(sys.Uid))
	if u, err := user.LookupId(name); err == nil {
		name = u.Username
	}
	return fmt.Errorf("run this as %s, who owns %s: runuser -u %s -- %s ...", name, filepath.Dir(db), name, os.Args[0])
}
