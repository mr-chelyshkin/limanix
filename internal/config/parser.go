package config

import (
	"bytes"
	"reflect"
	"unicode/utf8"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/pelletier/go-toml/v2"
)

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

func applyEntryDefaults(model reflect.Value, supplied map[string]any) {
	for index := range model.NumField() {
		var (
			item          = model.Type().Field(index)
			value         = model.Field(index)
			entry, exists = supplied[item.Tag.Get("toml")]
		)

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
