// +build !linux

package idtools

import "fmt"

// AddRemappedRootUser takes a name and finds an unused uid, gid pair
// and calls the appropriate helper function to add the group and then
// the user to the group in /etc/group and /etc/passwd respectively.
// This new user will will be used as the remapped uid/gid pair for root
func AddRemappedRootUser(name string) (int, int, error) {
	return -1, -1, fmt.Errorf("No support for adding user/group on this OS")
}
