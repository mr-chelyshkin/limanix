package domain

import (
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	vmNamePattern     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	usernamePattern   = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	moduleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	envNamePattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// ValidateText checks string boundaries without disclosing its content in errors.
func ValidateText(value string, allowEmpty bool) error {
	if !utf8.ValidString(value) {
		return ErrInvalidUTF8
	}

	if strings.ContainsRune(value, 0) {
		return ErrNULCharacter
	}

	if value == "" && !allowEmpty {
		return ErrEmptyValue
	}

	return nil
}

// NewVMName validates a lowercase host-local VM name.
func NewVMName(value string) (VMName, error) {
	if err := ValidateText(value, false); err != nil {
		return "", err
	}

	if !vmNamePattern.MatchString(value) {
		return "", ErrInvalidVMName
	}

	return VMName(value), nil
}

// NewUsername validates a guest username and rejects reserved accounts.
func NewUsername(value string) (Username, error) {
	if err := ValidateText(value, false); err != nil {
		return "", err
	}

	if !usernamePattern.MatchString(value) || value == "root" || value == "limanix-admin" {
		return "", ErrInvalidUsername
	}

	return Username(value), nil
}

// NewGuestPath validates and normalizes a guest path without parent traversal.
func NewGuestPath(value string) (GuestPath, error) {
	if err := ValidateText(value, false); err != nil {
		return "", err
	}

	if strings.ContainsAny(value, " \t\r\n") {
		return "", ErrGuestPathWhitespace
	}

	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "", ErrInvalidGuestPath
	}

	for _, component := range strings.Split(value, "/") {
		if component == ".." {
			return "", ErrInvalidGuestPath
		}
	}

	normalized := path.Clean(value)
	if normalized == "/" {
		return "", ErrGuestRoot
	}

	return GuestPath(normalized), nil
}

// NewModuleName validates the unqualified name of a NixOS module.
func NewModuleName(value string) (ModuleName, error) {
	if err := ValidateText(value, false); err != nil {
		return "", err
	}

	if len(value) > 63 || !moduleNamePattern.MatchString(value) {
		return "", ErrInvalidModuleName
	}

	return ModuleName(value), nil
}

// NewModuleID validates a built-in or third-party module reference.
func NewModuleID(value string) (ModuleID, error) {
	if _, err := NewModuleName(strings.TrimPrefix(value, "third-party:")); err != nil {
		return "", err
	}

	return ModuleID(value), nil
}

// Name returns the module name without its source namespace.
func (id ModuleID) Name() ModuleName {
	return ModuleName(strings.TrimPrefix(string(id), "third-party:"))
}

// IsThirdParty reports whether the module comes from the imported registry.
func (id ModuleID) IsThirdParty() bool {
	return strings.HasPrefix(string(id), "third-party:")
}

// NewEnvName validates an environment variable name.
func NewEnvName(value string) (EnvName, error) {
	if err := ValidateText(value, false); err != nil {
		return "", err
	}

	if !envNamePattern.MatchString(value) {
		return "", ErrInvalidEnvName
	}

	return EnvName(value), nil
}

// NewEnvValue validates literal text supported by guest environment files.
func NewEnvValue(value string) (EnvValue, error) {
	if err := ValidateText(value, true); err != nil {
		return "", err
	}

	for _, character := range value {
		if character == 0xFEFF ||
			(character >= 0xFDD0 && character <= 0xFDEF) ||
			character&0xFFFF == 0xFFFE ||
			character&0xFFFF == 0xFFFF {
			return "", ErrInvalidEnvValue
		}
	}

	return EnvValue(value), nil
}
