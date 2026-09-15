package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/sys/unix"
)

// Error identifies a public configuration field without exposing its value.
type Error struct {
	Field   string
	Message string
}

func (err *Error) Error() string             { return err.Field + ": " + err.Message }
func fieldError(field, message string) error { return &Error{Field: field, Message: message} }

var byteSizeType = reflect.TypeFor[domain.ByteSize]()

// Parse performs no filesystem access and leaves host paths and environment values literal.
func Parse(data []byte) (Config, error) {
	if !utf8.Valid(data) {
		return Config{}, fieldError("config", "expected UTF-8 text")
	}
	var supplied map[string]any
	if err := toml.Unmarshal(data, &supplied); err != nil {
		return Config{}, decodeError(err)
	}
	if err := checkShape(supplied, reflect.TypeFor[Config](), ""); err != nil {
		return Config{}, err
	}
	result := Default()
	// Explicit tables replace default collections, including empty [env].
	if _, exists := supplied["env"]; exists {
		result.Env = map[domain.EnvName]domain.EnvValue{}
	}
	decoder := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return Config{}, decodeError(err)
	}
	applyEntryDefaults(reflect.ValueOf(&result).Elem(), supplied)
	if err := Validate(result); err != nil {
		return Config{}, err
	}
	result.User.Home, _ = domain.NewGuestPath(string(result.User.Home))
	for index := range result.Mounts {
		result.Mounts[index].Target, _ = domain.NewGuestPath(string(result.Mounts[index].Target))
	}
	return result, nil
}

// Collection entries are newly allocated by the decoder; their optional fields
// receive the defaults declared in the model rather than top-level defaults.
func applyEntryDefaults(model reflect.Value, supplied map[string]any) {
	for index := range model.NumField() {
		item := model.Type().Field(index)
		value := model.Field(index)
		entry, exists := supplied[item.Tag.Get("toml")]
		if !exists {
			if declared, ok := item.Tag.Lookup("default"); ok {
				value.SetString(declared)
			}
			continue
		}
		if value.Kind() == reflect.Struct {
			applyEntryDefaults(value, entry.(map[string]any))
		}
		if isTableArray(value) {
			for entryIndex, table := range entry.([]any) {
				applyEntryDefaults(value.Index(entryIndex), table.(map[string]any))
			}
		}
	}
}

func decodeError(err error) error {
	var decode *toml.DecodeError
	if errors.As(err, &decode) {
		line, column := decode.Position()
		return fieldError("config", fmt.Sprintf("invalid TOML at line %d, column %d", line, column))
	}
	return fieldError("config", "invalid TOML")
}

func joinField(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

// checkShape uses model tags and Go types to provide precise diagnostics before
// the strict decoder runs. Decoder source excerpts are deliberately not returned.
func checkShape(value any, model reflect.Type, field string) error {
	if model == byteSizeType {
		text, ok := value.(string)
		if !ok {
			return fieldError(field, "expected a whole GiB size string")
		}
		if _, err := domain.ParseByteSize(text); err != nil {
			return fieldError(field, err.Error())
		}
		return nil
	}
	switch model.Kind() {
	case reflect.Struct:
		table, ok := value.(map[string]any)
		if !ok {
			return fieldError(field, "expected a table")
		}
		fields := map[string]reflect.StructField{}
		for index := range model.NumField() {
			item := model.Field(index)
			fields[item.Tag.Get("toml")] = item
		}
		for _, key := range sortedKeys(table) {
			if _, exists := fields[key]; !exists {
				return fieldError(joinField(field, key), "unknown field")
			}
		}
		for index := range model.NumField() {
			item := model.Field(index)
			name := item.Tag.Get("toml")
			entry, exists := table[name]
			if !exists {
				if item.Tag.Get("required") == "true" {
					return fieldError(joinField(field, name), "required field is missing")
				}
				continue
			}
			if err := checkShape(entry, item.Type, joinField(field, name)); err != nil {
				return err
			}
		}
	case reflect.Slice:
		entries, ok := value.([]any)
		if !ok {
			return fieldError(field, "expected an array")
		}
		for index, entry := range entries {
			if err := checkShape(entry, model.Elem(), fmt.Sprintf("%s[%d]", field, index)); err != nil {
				return err
			}
		}
	case reflect.Map:
		table, ok := value.(map[string]any)
		if !ok {
			return fieldError(field, "expected a table")
		}
		for _, key := range sortedKeys(table) {
			if _, err := domain.NewEnvName(key); err != nil {
				return fieldError(field, err.Error())
			}
			if err := checkShape(table[key], model.Elem(), joinField(field, key)); err != nil {
				return err
			}
		}
	case reflect.String:
		text, ok := value.(string)
		if !ok {
			return fieldError(field, "expected a string")
		}
		if err := validateString(reflect.ValueOf(text).Convert(model)); err != nil {
			return fieldError(field, err.Error())
		}
	case reflect.Int:
		if _, ok := value.(int64); !ok {
			return fieldError(field, "expected an integer")
		}
	case reflect.Bool:
		if _, ok := value.(bool); !ok {
			return fieldError(field, "expected a boolean")
		}
	default:
		return fieldError(field, "unsupported configuration field type")
	}
	return nil
}

func sortedKeys[V any](table map[string]V) []string {
	keys := make([]string, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validateString(value reflect.Value) error {
	text := value.String()
	switch value.Interface().(type) {
	case domain.VMName:
		_, err := domain.NewVMName(text)
		return err
	case domain.Username:
		_, err := domain.NewUsername(text)
		return err
	case domain.GuestPath:
		_, err := domain.NewGuestPath(text)
		return err
	case domain.ModuleID:
		_, err := domain.NewModuleID(text)
		return err
	case domain.EnvName:
		_, err := domain.NewEnvName(text)
		return err
	case domain.EnvValue:
		_, err := domain.NewEnvValue(text)
		return err
	case domain.Architecture:
		_, err := domain.NewArchitecture(text)
		return err
	default:
		return domain.ValidateText(text, false)
	}
}

func validateValue(value reflect.Value, field string) error {
	if value.Type() == byteSizeType {
		_, err := value.Interface().(domain.ByteSize).GiB()
		if err != nil {
			return fieldError(field, err.Error())
		}
		return nil
	}
	switch value.Kind() {
	case reflect.Struct:
		for index := range value.NumField() {
			item := value.Type().Field(index)
			child := joinField(field, item.Tag.Get("toml"))
			if choices := item.Tag.Get("choices"); choices != "" {
				if !slices.Contains(strings.Split(choices, ","), value.Field(index).String()) {
					return fieldError(child, "expected one of "+strings.ReplaceAll(choices, ",", ", "))
				}
			}
			if err := validateValue(value.Field(index), child); err != nil {
				return err
			}
		}
	case reflect.String:
		if err := validateString(value); err != nil {
			return fieldError(field, err.Error())
		}
	case reflect.Slice:
		for index := range value.Len() {
			if err := validateValue(value.Index(index), fmt.Sprintf("%s[%d]", field, index)); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range sortedMapKeys(value) {
			if err := validateValue(key, field); err != nil {
				return err
			}
			if err := validateValue(value.MapIndex(key), joinField(field, key.String())); err != nil {
				return err
			}
		}
	}
	return nil
}

func isWithin(candidate, root string) bool {
	return candidate == root || strings.HasPrefix(candidate, root+"/")
}

func checkedGuestTarget(value domain.GuestPath, field string) (string, error) {
	normalized, err := domain.NewGuestPath(string(value))
	if err != nil {
		return "", fieldError(field, err.Error())
	}
	target := string(normalized)
	for _, root := range []string{"/etc", "/boot", "/usr", "/var", "/nix", "/run", "/dev", "/proc", "/sys", "/bin", "/sbin", "/mnt/limanix", "/home/limanix-admin"} {
		if isWithin(target, root) {
			return "", fieldError(field, "destination is reserved for the guest system")
		}
	}
	if isWithin("/home/limanix-admin", target) {
		return "", fieldError(field, "destination is reserved for the guest system")
	}
	return target, nil
}

// Validate checks domain values and cross-field policy without consulting the module registry.
func Validate(config Config) error {
	if err := validateValue(reflect.ValueOf(config), ""); err != nil {
		return err
	}
	if config.SchemaVersion != 1 {
		return fieldError("schema_version", "only version 1 is supported")
	}
	if config.Resources.CPU <= 0 {
		return fieldError("resources.cpu", "must be positive")
	}
	if path.Clean(config.Home.Root) == "/" {
		return fieldError("home.root", "the host root directory is not allowed")
	}
	for _, protocol := range []struct {
		name  string
		ports []int
	}{{"tcp", config.Network.Ports.TCP}, {"udp", config.Network.Ports.UDP}} {
		for index, port := range protocol.ports {
			if port < 1 || port > 65535 {
				return fieldError(fmt.Sprintf("network.ports.%s[%d]", protocol.name, index), "port must be between 1 and 65535")
			}
		}
	}
	seen := map[domain.ModuleID]bool{}
	for index, module := range config.NixOS.Modules {
		if seen[module] {
			return fieldError(fmt.Sprintf("nixos.modules[%d]", index), "duplicate module ID")
		}
		seen[module] = true
	}
	userHome, err := checkedGuestTarget(config.User.Home, "user.home")
	if err != nil {
		return err
	}
	targets := []string{}
	for index, mount := range config.Mounts {
		field := fmt.Sprintf("mounts[%d].target", index)
		target, err := checkedGuestTarget(mount.Target, field)
		if err != nil {
			return err
		}
		if isWithin(userHome, target) {
			return fieldError(field, "would hide the managed user home")
		}
		for _, previous := range targets {
			if isWithin(target, previous) || isWithin(previous, target) {
				return fieldError(field, "overlaps another explicit mount")
			}
		}
		targets = append(targets, target)
	}
	return nil
}

// Load reads a regular UTF-8 TOML file and resolves host paths relative to its real location.
// Missing mount sources are accepted; environment variable references remain literal.
func Load(filename string) (Config, error) {
	if err := domain.ValidateText(filename, false); err != nil {
		return Config{}, fieldError("config", "invalid configuration file path")
	}
	resolved, err := filesystem.Resolve(filename)
	if err != nil {
		return Config{}, fieldError("config", "configuration file cannot be read")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return Config{}, fieldError("config", "configuration file cannot be read")
	}
	if !info.Mode().IsRegular() {
		return Config{}, fieldError("config", "expected a regular TOML file")
	}
	file, err := filesystem.OpenRegular(resolved, unix.O_RDONLY, 0)
	if err != nil {
		return Config{}, fieldError("config", "configuration file cannot be read")
	}
	data, readErr := io.ReadAll(file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return Config{}, fieldError("config", "configuration file cannot be read")
	}
	config, err := Parse(data)
	if err != nil {
		return Config{}, err
	}
	parent := filepath.Dir(resolved)
	config.Home.Root, err = canonicalHostPath(config.Home.Root, parent)
	if err != nil {
		return Config{}, fieldError("home.root", "host path cannot be resolved")
	}
	if config.Home.Root == "/" {
		return Config{}, fieldError("home.root", "the host root directory is not allowed")
	}
	for index := range config.Mounts {
		config.Mounts[index].Source, err = canonicalHostPath(config.Mounts[index].Source, parent)
		if err != nil {
			return Config{}, fieldError(fmt.Sprintf("mounts[%d].source", index), "host path cannot be resolved")
		}
	}
	return config, nil
}

func canonicalHostPath(value, parent string) (string, error) {
	expanded, err := filesystem.ExpandHome(value)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(expanded) {
		expanded = parent + string(filepath.Separator) + expanded
	}
	resolved, err := filesystem.Resolve(expanded)
	if err != nil {
		return "", err
	}
	if err := domain.ValidateText(resolved, false); err != nil {
		return "", err
	}
	return resolved, nil
}
