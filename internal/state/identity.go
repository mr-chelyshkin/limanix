package state

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// IDLength is the number of hexadecimal characters in identity and generation IDs.
const IDLength = 12

// Identity defines a VM and its exact managed host home. Saved identities cannot change.
type Identity struct {
	UserHome  domain.GuestPath    `json:"user_home"`
	Username  domain.Username     `json:"username"`
	Arch      domain.Architecture `json:"arch"`
	Name      domain.VMName       `json:"name"`
	CreatedAt string              `json:"created_at"`
	HomeRoot  string              `json:"home_root"`
	Home      string              `json:"home"`
	ID        string              `json:"id"`
}

// LimaName returns the unique backend name derived from the immutable identity.
func (i Identity) LimaName() string { return "limanix-" + string(i.Name) + "-" + i.ID }

var identifierPattern = regexp.MustCompile(fmt.Sprintf(`^[a-f0-9]{%d}$`, IDLength))

// HomePath validates the allocation fields and derives root/<name>-<id>.
func HomePath(root string, name domain.VMName, identifier string) (string, error) {
	if _, err := domain.NewVMName(string(name)); err != nil {
		return "", err
	}
	if !identifierPattern.MatchString(identifier) {
		return "", errors.New("invalid identity id")
	}
	if domain.ValidateText(root, false) != nil || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", errors.New("invalid managed-home root")
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
		return "", "", errors.New("invalid managed-home identity")
	}
	return i.HomeRoot, home, nil
}

// Validate checks an identity without consulting a user configuration schema.
func (i Identity) Validate() error {
	_, err := validatedIdentity(i)
	return err
}

func validatedIdentity(i Identity) (Identity, error) {
	if _, err := domain.NewVMName(string(i.Name)); err != nil {
		return Identity{}, err
	}
	if _, err := domain.NewArchitecture(string(i.Arch)); err != nil {
		return Identity{}, err
	}
	if _, err := domain.NewUsername(string(i.Username)); err != nil {
		return Identity{}, err
	}

	userHome, err := domain.NewGuestPath(string(i.UserHome))
	if err != nil {
		return Identity{}, err
	}

	i.UserHome = userHome
	if domain.ValidateText(i.CreatedAt, false) != nil {
		return Identity{}, errors.New("invalid created_at")
	}
	if _, _, err = i.HomePaths(); err != nil {
		return Identity{}, err
	}
	return i, nil
}
