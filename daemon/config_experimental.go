// +build experimental

package daemon

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/docker/pkg/idtools"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/opencontainers/runc/libcontainer/user"
)

func (config *Config) attachExperimentalFlags(cmd *flag.FlagSet, usageFn func(string) string) {
	cmd.StringVar(&config.DefaultNetwork, []string{"-default-network"}, "", usageFn("Set default network"))
	cmd.StringVar(&config.RemappedRoot, []string{"-root"}, "", usageFn("User/Group setting for container root"))
}

const (
	defaultIDSpecifier string = "default"
	defaultRemappedID  string = "dockroot"
)

// Parse the remapped root (user namespace) option, which can be one of:
//   username            - valid username from /etc/passwd
//   username:groupname  - valid username; valid groupname from /etc/group
//   uid                 - 32-bit unsigned int valid Linux UID value
//   uid:gid             - uid value; 32-bit unsigned int Linux GID value
//
//  If no groupname is specified, and a username is specified, an attempt
//  will be made to lookup a gid for that username as a groupname
//
//  If names are used, they are mapped to the appropriate 32-bit unsigned int
func parseRemappedRoot(usergrp string) (int, int, error) {

	var userID, groupID int

	idparts := strings.Split(usergrp, ":")
	if len(idparts) > 2 {
		return 0, 0, fmt.Errorf("Invalid user/group specification in --root: %q", usergrp)
	}

	if uid, err := strconv.ParseInt(idparts[0], 10, 32); err == nil {
		// must be a uid; take it as valid
		userID = int(uid)
		if len(idparts) == 1 {
			// if the uid was numeric and no gid was specified, take the uid as the gid
			groupID = userID
		}
	} else {
		lookupID := idparts[0]
		// special case: if the user specified "default", they want Docker to create or
		// use (after creation) the "dockroot" user/group for root remapping
		if lookupID == defaultIDSpecifier {
			lookupID = defaultRemappedID
		}
		luser, err := user.LookupUser(lookupID)
		if err != nil && idparts[0] != defaultIDSpecifier {
			// error if the name requested isn't the special "dockroot" ID
			return 0, 0, fmt.Errorf("Error during uid lookup for %q: %v", lookupID, err)
		} else if err != nil {
			// special case-- if the username == "default", then we have been asked
			// to create a new uid/gid pair in /etc/{passwd,group} to use as the remapped
			// root UID and GID for this daemon instance
			newUID, newGID, err := idtools.AddRemappedRootUser(defaultRemappedID)
			if err == nil {
				return newUID, newGID, nil
			}
			return 0, 0, fmt.Errorf("Error during %q user creation: %v", defaultRemappedID, err)
		}
		userID = luser.Uid
		if len(idparts) == 1 {
			// we only have a string username, and no group specified; look up gid from username as group
			group, err := user.LookupGroup(lookupID)
			if err != nil {
				return 0, 0, fmt.Errorf("Error during gid lookup for %q: %v", lookupID, err)
			}
			groupID = group.Gid
		}
	}

	if len(idparts) == 2 {
		// groupname or gid is separately specified and must be resolved
		// to a unsigned 32-bit gid
		if gid, err := strconv.ParseInt(idparts[1], 10, 32); err == nil {
			// must be a gid, take it as valid
			groupID = int(gid)
		} else {
			// not a number; attempt a lookup
			group, err := user.LookupGroup(idparts[1])
			if err != nil {
				return 0, 0, fmt.Errorf("Error during gid lookup for %q: %v", idparts[1], err)
			}
			groupID = group.Gid
		}
	}
	return userID, groupID, nil
}
