package config

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

var byteSizeType = reflect.TypeFor[domain.ByteSize]()

func joinField(parent, name string) string {
	if parent == "" {
		return name
	}

	return parent + "." + name
}

func checkShape(value any, model reflect.Type, field string) error {
	if model == byteSizeType {
		return checkSize(value, field)
	}

	switch model.Kind() {
	case reflect.Struct:
		return checkTable(value, model, field)
	case reflect.Slice:
		return checkArray(value, model, field)
	case reflect.Map:
		return checkEnvironment(value, model, field)
	case reflect.String:
		text, ok := value.(string)
		if !ok {
			return fieldError(field, "expected a string")
		}

		if err := validateString(reflect.ValueOf(text).Convert(model)); err != nil {
			return wrapField(field, err.Error(), err)
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

func checkSize(value any, field string) error {
	text, ok := value.(string)
	if !ok {
		return fieldError(field, "expected a whole GiB size string")
	}

	if _, err := domain.ParseByteSize(text); err != nil {
		return wrapField(field, err.Error(), err)
	}

	return nil
}

// checkTable rejects unknown fields before traversing model fields in declaration order.
func checkTable(value any, model reflect.Type, field string) error {
	table, ok := value.(map[string]any)
	if !ok {
		return fieldError(field, "expected a table")
	}

	fields := make(map[string]reflect.StructField, model.NumField())
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
		var (
			item          = model.Field(index)
			name          = item.Tag.Get("toml")
			child         = joinField(field, name)
			entry, exists = table[name]
		)

		if !exists {
			if item.Tag.Get("required") == "true" {
				return fieldError(child, "required field is missing")
			}

			continue
		}

		if err := checkShape(entry, item.Type, child); err != nil {
			return err
		}
	}

	return nil
}

func checkArray(value any, model reflect.Type, field string) error {
	entries, ok := value.([]any)
	if !ok {
		return fieldError(field, "expected an array")
	}

	for index, entry := range entries {
		if err := checkShape(entry, model.Elem(), fmt.Sprintf("%s[%d]", field, index)); err != nil {
			return err
		}
	}

	return nil
}

func checkEnvironment(value any, model reflect.Type, field string) error {
	table, ok := value.(map[string]any)
	if !ok {
		return fieldError(field, "expected a table")
	}

	for _, key := range sortedKeys(table) {
		if _, err := domain.NewEnvName(key); err != nil {
			return wrapField(field, err.Error(), err)
		}

		if err := checkShape(table[key], model.Elem(), joinField(field, key)); err != nil {
			return err
		}
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
			return wrapField(field, err.Error(), err)
		}

		return nil
	}

	switch value.Kind() {
	case reflect.Struct:
		for index := range value.NumField() {
			var (
				item  = value.Type().Field(index)
				child = joinField(field, item.Tag.Get("toml"))
			)

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
			return wrapField(field, err.Error(), err)
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

func sortedMapKeys(value reflect.Value) []reflect.Value {
	keys := value.MapKeys()
	sort.Slice(keys, func(a, b int) bool {
		return keys[a].String() < keys[b].String()
	})
	return keys
}
