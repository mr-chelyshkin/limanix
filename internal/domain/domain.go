// Package domain defines values shared by configuration, persistent state and VM integrations.
package domain

import (
	"encoding/json"
	"errors"
	"math"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type (
	VMName       string
	Username     string
	GuestPath    string
	ModuleName   string
	ModuleID     string
	EnvName      string
	EnvValue     string
	Architecture string
	ByteSize     int64
)

const (
	ARM64 Architecture = "arm64"
	AMD64 Architecture = "amd64"
	GiB   int64        = 1 << 30
)

// Architectures lists the public architecture choices in documentation order.
func Architectures() []Architecture { return []Architecture{ARM64, AMD64} }

var (
	vmNamePattern     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	usernamePattern   = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	moduleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	envNamePattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	sizePattern       = regexp.MustCompile(`^[1-9][0-9]*GiB$`)
)

// ValidateText checks string boundaries without disclosing its content in errors.
func ValidateText(value string, allowEmpty bool) error {
	if !utf8.ValidString(value) {
		return errors.New("expected UTF-8 text")
	}
	if strings.ContainsRune(value, 0) {
		return errors.New("must not contain NUL characters")
	}
	if value == "" && !allowEmpty {
		return errors.New("must not be empty")
	}
	return nil
}

func NewVMName(value string) (VMName, error) {
	if err := ValidateText(value, false); err != nil {
		return "", err
	}
	if !vmNamePattern.MatchString(value) {
		return "", errors.New("expected a lowercase hostname label of 1 to 63 characters")
	}
	return VMName(value), nil
}

func NewUsername(value string) (Username, error) {
	if err := ValidateText(value, false); err != nil {
		return "", err
	}
	if !usernamePattern.MatchString(value) || value == "root" || value == "limanix-admin" {
		return "", errors.New("expected a non-reserved Linux username of 1 to 32 characters")
	}
	return Username(value), nil
}

func NewGuestPath(value string) (GuestPath, error) {
	if err := ValidateText(value, false); err != nil {
		return "", err
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return "", errors.New("guest mount paths cannot contain spaces, tabs, or line breaks because NixOS Lima does not escape them in fstab")
	}
	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "", errors.New("expected an absolute guest path without '..'")
	}
	for _, component := range strings.Split(value, "/") {
		if component == ".." {
			return "", errors.New("expected an absolute guest path without '..'")
		}
	}
	normalized := path.Clean(value)
	if normalized == "/" {
		return "", errors.New("the guest root directory is not allowed")
	}
	return GuestPath(normalized), nil
}

func NewModuleName(value string) (ModuleName, error) {
	if err := ValidateText(value, false); err != nil {
		return "", err
	}
	if len(value) > 63 || !moduleNamePattern.MatchString(value) {
		return "", errors.New("expected a module name of 1 to 63 lowercase letters, digits, and single hyphens, starting with a letter")
	}
	return ModuleName(value), nil
}

func NewModuleID(value string) (ModuleID, error) {
	if _, err := NewModuleName(strings.TrimPrefix(value, "third-party:")); err != nil {
		return "", err
	}
	return ModuleID(value), nil
}

func (id ModuleID) Name() ModuleName {
	return ModuleName(strings.TrimPrefix(string(id), "third-party:"))
}
func (id ModuleID) IsThirdParty() bool { return strings.HasPrefix(string(id), "third-party:") }

func NewEnvName(value string) (EnvName, error) {
	if err := ValidateText(value, false); err != nil {
		return "", err
	}
	if !envNamePattern.MatchString(value) {
		return "", errors.New("expected a POSIX environment variable name")
	}
	return EnvName(value), nil
}

func NewEnvValue(value string) (EnvValue, error) {
	if err := ValidateText(value, true); err != nil {
		return "", err
	}
	for _, character := range value {
		if character == 0xFEFF ||
			(character >= 0xFDD0 && character <= 0xFDEF) ||
			character&0xFFFF == 0xFFFE ||
			character&0xFFFF == 0xFFFF {
			return "", errors.New("contains a character unsupported by guest environment files")
		}
	}
	return EnvValue(value), nil
}

func NewArchitecture(value string) (Architecture, error) {
	switch Architecture(value) {
	case ARM64, AMD64:
		return Architecture(value), nil
	default:
		return "", errors.New("expected one of arm64, amd64")
	}
}

func (architecture Architecture) LimaArch() (string, error) {
	switch architecture {
	case ARM64:
		return "aarch64", nil
	case AMD64:
		return "x86_64", nil
	default:
		return "", errors.New("expected one of arm64, amd64")
	}
}

func NewByteSize(value int64) (ByteSize, error) {
	if value <= 0 {
		return 0, errors.New("expected a positive integer byte count")
	}
	return ByteSize(value), nil
}

func ParseByteSize(value string) (ByteSize, error) {
	if !sizePattern.MatchString(value) {
		return 0, errors.New("expected a positive whole GiB size, such as 8GiB")
	}
	amount, err := strconv.ParseInt(strings.TrimSuffix(value, "GiB"), 10, 64)
	if err != nil || amount > math.MaxInt64/GiB {
		return 0, errors.New("size exceeds the supported integer limit")
	}
	return NewByteSize(amount * GiB)
}

func (size ByteSize) GiB() (string, error) {
	if size <= 0 || int64(size)%GiB != 0 {
		return "", errors.New("byte count cannot be represented as a positive whole GiB size")
	}
	return strconv.FormatInt(int64(size)/GiB, 10) + "GiB", nil
}

func (size ByteSize) MarshalText() ([]byte, error) {
	value, err := size.GiB()
	return []byte(value), err
}

func (size *ByteSize) UnmarshalText(data []byte) error {
	parsed, err := ParseByteSize(string(data))
	if err != nil {
		return err
	}
	*size = parsed
	return nil
}

func (size ByteSize) MarshalJSON() ([]byte, error) {
	value, err := size.GiB()
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func (size *ByteSize) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return errors.New("expected a whole GiB size string")
	}
	return size.UnmarshalText([]byte(value))
}
