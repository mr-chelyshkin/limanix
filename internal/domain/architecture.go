package domain

// Architecture is a supported guest CPU architecture in Go notation.
type Architecture string

const (
	ARM64 Architecture = "arm64"
	AMD64 Architecture = "amd64"
)

// Architectures lists the public architecture choices in documentation order.
func Architectures() []Architecture {
	return []Architecture{ARM64, AMD64}
}

// NewArchitecture validates the public architecture spelling.
func NewArchitecture(value string) (Architecture, error) {
	switch Architecture(value) {
	case ARM64, AMD64:
		return Architecture(value), nil
	default:
		return "", ErrInvalidArchitecture
	}
}

// LimaArch returns the spelling used by Lima's templates and guest-agent archives.
func (architecture Architecture) LimaArch() (string, error) {
	switch architecture {
	case ARM64:
		return "aarch64", nil
	case AMD64:
		return "x86_64", nil
	default:
		return "", ErrInvalidArchitecture
	}
}
