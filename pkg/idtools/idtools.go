package idtools

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/docker/docker/pkg/system"
	"github.com/docker/docker/vendor/src/github.com/opencontainers/runc/libcontainer/user"
)

// IDMap contains a single entry for user namespace range remapping. An array
// of IDMap entries represents the structure that will be provided to the Linux
// kernel for creating a user namespace.
type IDMap struct {
	ContainerID int `json:"container_id"`
	HostID      int `json:"host_id"`
	Size        int `json:"size"`
}

const (
	subuidFileName string = "/etc/subuid"
	subgidFileName string = "/etc/subgid"
)

// MkdirAllAs creates a directory (include any along the path) and then modifies
// ownership to the requested uid/gid.  If the directory already exists, this
// function will still change ownership to the requested uid/gid pair.
func MkdirAllAs(path string, mode os.FileMode, ownerUID, ownerGID int) error {
	return mkdirAs(path, mode, ownerUID, ownerGID, true)
}

// MkdirAs creates a directory and then modifies ownership to the requested uid/gid.
// If the directory already exists, this function still changes ownership
func MkdirAs(path string, mode os.FileMode, ownerUID, ownerGID int) error {
	return mkdirAs(path, mode, ownerUID, ownerGID, false)
}

func mkdirAs(path string, mode os.FileMode, ownerUID, ownerGID int, mkAll bool) error {
	if mkAll {
		if err := system.MkdirAll(path, mode); err != nil && !os.IsExist(err) {
			return err
		}
	} else {
		if err := os.Mkdir(path, mode); err != nil && !os.IsExist(err) {
			return err
		}
	}
	// even if it existed, we will chown to change ownership as requested
	if err := os.Chown(path, ownerUID, ownerGID); err != nil {
		return err
	}
	return nil
}

// GetRootUIDGID retrieves the remapped root uid/gid pair from the set of maps.
// If the maps are empty, then the root uid/gid will default to "real" 0/0
func GetRootUIDGID(uidMap, gidMap []IDMap) (int, int, error) {
	var uid, gid int

	if uidMap != nil {
		xUID, err := ToHost(0, uidMap)
		if err != nil {
			return -1, -1, err
		}
		uid = xUID
	}
	if gidMap != nil {
		xGID, err := ToHost(0, gidMap)
		if err != nil {
			return -1, -1, err
		}
		gid = xGID
	}
	return uid, gid, nil
}

// ToContainer takes an id mapping, and uses it to translate a
// host ID to the remapped ID. If no map is provided, then the translation
// assumes a 1-to-1 mapping and returns the passed in id
func ToContainer(hostID int, idMap []IDMap) (int, error) {
	if idMap == nil {
		return hostID, nil
	}
	for _, m := range idMap {
		if (hostID >= m.HostID) && (hostID <= (m.HostID + m.Size - 1)) {
			contID := m.ContainerID + (hostID - m.HostID)
			return contID, nil
		}
	}
	return -1, fmt.Errorf("Host ID %d cannot be mapped to a container ID", hostID)
}

// ToHost takes an id mapping and a remapped ID, and translates the
// ID to the mapped host ID. If no map is provided, then the translation
// assumes a 1-to-1 mapping and returns the passed in id #
func ToHost(contID int, idMap []IDMap) (int, error) {
	if idMap == nil {
		return contID, nil
	}
	for _, m := range idMap {
		if (contID >= m.ContainerID) && (contID <= (m.ContainerID + m.Size - 1)) {
			hostID := m.HostID + (contID - m.ContainerID)
			return hostID, nil
		}
	}
	return -1, fmt.Errorf("Container ID %d cannot be mapped to a host ID", contID)
}

// CreateIDMappings takes a requested user and group name and
// using the data from /etc/sub{uid,gid} ranges, creates the
// proper uid and gid remapping ranges for that user/group pair
func CreateIDMappings(username, groupname string) ([]IDMap, []IDMap, error) {
	uidMap, err := parseSubuid(username)
	if err != nil {
		return nil, nil, err
	}
	gidMap, err := parseSubgid(groupname)
	if err != nil {
		return nil, nil, err
	}

	//now need to create a special idmap for root
	return createIDMap(username, uidMap), createIDMap(username, gidMap), nil
}

func createIDMap(username string, idMap []IDMap) []IDMap {
	usr,err := user.LookupUser(username)
	if err != nil {
		return nil
	}
	// create a single continguous map of 65536 length (even if
	// the sub{uid,gid} range is larger) starting at root (0)
	idMap = append(idMap, IDMap{
		ContainerID: 0,
		HostID:      usr.Uid,
		Size:        1,
	})
	return idMap
}

func parseSubuid(username string) ([]IDMap, error) {
	return parseSubidFile(subuidFileName, username)
}

func parseSubgid(username string) ([]IDMap, error) {
	return parseSubidFile(subgidFileName, username)
}

func parseSubidFile(path, username string) ([]IDMap, error) {
	idMap := []IDMap{}
	subidFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer subidFile.Close()

	s := bufio.NewScanner(subidFile)
	for s.Scan() {
		if err := s.Err(); err != nil {
			return nil, err
		}

		text := strings.TrimSpace(s.Text())
		if text == "" {
			continue
		}
		parts := strings.Split(text, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("Cannot parse subuid/gid information: Format not correct for %s file", path)
		}
		usr,err := user.LookupUser(username)
		if err != nil {
			return nil, fmt.Errorf("Error parsing user %s: %v", username, err)
		}
		if parts[0] == username {
			// return the first entry for a user; ignores potential for multiple ranges per user
			startid, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, fmt.Errorf("String to int conversion failed during subuid/gid parsing of %s: %v", path, err)
			}
			length, err := strconv.Atoi(parts[2])
			if err != nil {
				return nil, fmt.Errorf("String to int conversion failed during subuid/gid parsing of %s: %v", path, err)
			}
			idMap = append(idMap, IDMap{
				ContainerID: usr.Uid,
				HostID:      startid,
				Size:        int(math.Min(float64(length), float64(length))),
			})
		}
	}
	return idMap, nil
}
