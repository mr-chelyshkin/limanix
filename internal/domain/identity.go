package domain

import (
	"fmt"
	"path/filepath"
	"regexp"
)

// IDLength is the number of hexadecimal characters in identity and generation IDs.
const IDLength = 12

var identifierPattern = regexp.MustCompile(fmt.Sprintf(`^[a-f0-9]{%d}$`, IDLength))

// Identity defines a VM and its exact managed host home.
// Persistence rejects changes to a saved identity, including CreatedAt and the allocation paths.
type Identity struct {
	UserHome  GuestPath    `json:"user_home"`
	Username  Username     `json:"username"`
	Arch      Architecture `json:"arch"`
	Name      VMName       `json:"name"`
	CreatedAt string       `json:"created_at"`
	HomeRoot  string       `json:"home_root"`
	Home      string       `json:"home"`
	ID        string       `json:"id"`
}

// LimaName returns the unique backend name derived from the immutable identity.
func (i Identity) LimaName() string {
	return "limanix-" + string(i.Name) + "-" + i.ID
}

// HomePath validates the allocation fields and derives root/<name>-<id>.
func HomePath(root string, name VMName, identifier string) (string, error) {
	if _, err := NewVMName(string(name)); err != nil {
		return "", err
	}

	if !identifierPattern.MatchString(identifier) {
		return "", ErrInvalidID
	}

	if ValidateText(root, false) != nil || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", ErrInvalidHomeRoot
	}

	return filepath.Join(root, string(name)+"-"+identifier), nil
}

// HomePaths validates that the saved home is exactly its recorded allocation.
func (i Identity) HomePaths() (root, home string, err error) {
	home, err = HomePath(i.HomeRoot, i.Name, i.ID)
	if err != nil {
		return "", "", err
	}

	if home != i.Home {
		return "", "", ErrInvalidHomeIdentity
	}

	return i.HomeRoot, home, nil
}

// Validate checks an identity without consulting a user configuration schema.
func (i Identity) Validate() error {
	_, err := i.Normalized()
	return err
}

// Normalized validates saved ownership and returns its canonical guest path.
// It does not apply defaults from the current configuration schema.
func (i Identity) Normalized() (Identity, error) {
	if _, err := NewVMName(string(i.Name)); err != nil {
		return Identity{}, err
	}

	if _, err := NewArchitecture(string(i.Arch)); err != nil {
		return Identity{}, err
	}

	if _, err := NewUsername(string(i.Username)); err != nil {
		return Identity{}, err
	}

	userHome, err := NewGuestPath(string(i.UserHome))
	if err != nil {
		return Identity{}, err
	}

	i.UserHome = userHome
	if ValidateText(i.CreatedAt, false) != nil {
		return Identity{}, ErrInvalidCreatedAt
	}

	if _, _, err = i.HomePaths(); err != nil {
		return Identity{}, err
	}

	return i, nil
}

// ValidIdentifier reports whether an ID uses the persisted identity format.
func ValidIdentifier(value string) bool {
	return identifierPattern.MatchString(value)
}
