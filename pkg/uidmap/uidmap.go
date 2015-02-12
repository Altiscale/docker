package uidmap

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/libcontainer/user"
)

func xlateOneFile(path string, finfo os.FileInfo, containerRoot uint32, inverse bool) error {
	uid := uint32(finfo.Sys().(*syscall.Stat_t).Uid)
	gid := uint32(finfo.Sys().(*syscall.Stat_t).Gid)
	mode := finfo.Mode()

	if (uid == containerRoot || gid == containerRoot) && !inverse {
		fmt.Errorf("Warning: UID of an existing file (%s) is docker-root")
	}
	if ((uid == 0 || gid == 0) && !inverse) || ((uid == containerRoot || gid == containerRoot) && inverse) {
		newUid := uid
		newGid := gid
		if uid == 0 && !inverse {
			newUid = containerRoot
		}
		if gid == 0 && !inverse {
			newGid = containerRoot
		}
		if uid == containerRoot && inverse {
			newUid = 0
		}
		if gid == containerRoot && inverse {
			newGid = 0
		}
		if err := os.Lchown(path, int(newUid), int(newGid)); err != nil {
			return fmt.Errorf("Cannot chown %s: %s", path, err)
		}
		if finfo.Mode() & os.ModeSymlink != os.ModeSymlink {
			if err := os.Chmod(path, mode); err != nil {
				return fmt.Errorf("Cannot chmod %s: %s", path, err)
			}
		}
	}

	return nil
}

func xlateUidsRecursive(base string, containerRoot uint32, inverse bool) error {
	f, err := os.Open(base)
	if err != nil {
		return err
	}

	list, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return err
	}

	for _, finfo := range list {
		path := filepath.Join(base, finfo.Name())
		if finfo.IsDir() {
			if err := xlateUidsRecursive(path, containerRoot, inverse); err != nil {
				log.Debugf("xlateUidsRecursive: %s", err)
				return err
			}
		}
		if err := xlateOneFile(path, finfo, containerRoot, inverse); err != nil {
			return err
		}
	}

	return nil
}

// Chown any root files to docker-root
func XlateUids(root string, inverse bool) error {
	containerRoot, err := ContainerRootUid()
	if err != nil {
		return err
	}
	if err := xlateUidsRecursive(root, containerRoot, inverse); err != nil {
		return err
	}
	finfo, err := os.Stat(root)
	if err != nil {
		return err
	}
	if err := xlateOneFile(root, finfo, containerRoot, inverse); err != nil {
		return err
	}

	return nil
}

// Get the uid of docker-root user
func ContainerRootUid() (uint32, error) {
	// Set up defaults.
	defaultExecUser := user.ExecUser{
		Uid:  syscall.Getuid(),
		Gid:  syscall.Getgid(),
		Home: "/",
	}

	passwdPath, err := user.GetPasswdPath()
	if err != nil {
		return 0, err
	}

	groupPath, err := user.GetGroupPath()
	if err != nil {
		return 0, err
	}

	execUser, err := user.GetExecUserPath("docker-root", &defaultExecUser, passwdPath, groupPath)
	if err != nil {
		return 0, err
	}

	return uint32(execUser.Uid), nil
}

// Get the highest uid on the host from /proc
func HostMaxUid() (uint32, error) {
	file, err := os.Open("/proc/self/uid_map")
	if err != nil {
		return 0, err
	}
	defer file.Close()

	uidMapString := make([]byte, 100)
	_, err = file.Read(uidMapString)
	if err != nil {
		return 0, err
	}

	var tmp, maxUid uint32
	fmt.Sscanf(string(uidMapString), "%d %d %d", &tmp, &tmp, &maxUid)

	return maxUid, nil
}
