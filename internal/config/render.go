package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// RenderExample renders the public defaults from the same model used by parsing.
func RenderExample() ([]byte, error) {
	return Render(Default())
}

// Render returns deterministic commented TOML for a validated configuration.
func Render(config Config) ([]byte, error) {
	if err := Validate(config); err != nil {
		return nil, err
	}

	var output strings.Builder

	writeComments(&output, "Default Limanix configuration for a development sandbox.")
	writeComments(&output, "Edit these values to match your environment.")
	output.WriteByte('\n')

	if err := renderModel(&output, reflect.ValueOf(config), ""); err != nil {
		return nil, err
	}

	return []byte(output.String()), nil
}

func writeComments(output *strings.Builder, description string) {
	line := "#"

	for _, word := range strings.Fields(description) {
		if len(line)+len(word)+1 > 88 {
			output.WriteString(line + "\n")
			line = "#"
		}

		line += " " + word
	}

	if line != "#" {
		output.WriteString(line + "\n")
	}
}

func isTableArray(value reflect.Value) bool {
	return value.Kind() == reflect.Slice && value.Type().Elem().Kind() == reflect.Struct
}

func renderModel(output *strings.Builder, model reflect.Value, prefix string) error {
	for index := range model.NumField() {
		var (
			item  = model.Type().Field(index)
			value = model.Field(index)
		)

		if value.Kind() == reflect.Struct || value.Kind() == reflect.Map || (isTableArray(value) && value.Len() > 0) {
			continue
		}

		encoded, err := tomlLiteral(value)
		if err != nil {
			return err
		}

		writeComments(output, item.Tag.Get("doc"))
		output.WriteString(item.Tag.Get("toml") + " = " + encoded + "\n")
	}

	for index := range model.NumField() {
		var (
			item  = model.Type().Field(index)
			value = model.Field(index)
			name  = joinField(prefix, item.Tag.Get("toml"))
		)

		switch {
		case value.Kind() == reflect.Struct:
			output.WriteByte('\n')
			writeComments(output, item.Tag.Get("doc"))
			output.WriteString("[" + name + "]\n")
			if err := renderModel(output, value, name); err != nil {
				return err
			}
		case value.Kind() == reflect.Map:
			output.WriteByte('\n')
			writeComments(output, item.Tag.Get("doc"))
			output.WriteString("[" + name + "]\n")

			for _, key := range sortedMapKeys(value) {
				encoded, err := tomlLiteral(value.MapIndex(key))
				if err != nil {
					return err
				}

				output.WriteString(key.String() + " = " + encoded + "\n")
			}
		case isTableArray(value) && value.Len() > 0:
			for entry := range value.Len() {
				output.WriteByte('\n')
				writeComments(output, item.Tag.Get("doc"))
				output.WriteString("[[" + name + "]]\n")

				if err := renderModel(output, value.Index(entry), name); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func tomlLiteral(value reflect.Value) (string, error) {
	if value.Type() == byteSizeType {
		text, err := value.Interface().(domain.ByteSize).GiB()
		if err != nil {
			return "", err
		}

		return quoteTOMLString(text), nil
	}

	switch value.Kind() {
	case reflect.String:
		return quoteTOMLString(value.String()), nil
	case reflect.Int, reflect.Int64:
		return strconv.FormatInt(value.Int(), 10), nil
	case reflect.Bool:
		return strconv.FormatBool(value.Bool()), nil
	case reflect.Slice:
		entries := make([]string, value.Len())

		for index := range value.Len() {
			var err error

			entries[index], err = tomlLiteral(value.Index(index))
			if err != nil {
				return "", err
			}
		}

		return "[" + strings.Join(entries, ", ") + "]", nil
	default:
		return "", fmt.Errorf("cannot render TOML type %s", value.Type())
	}
}

func quoteTOMLString(value string) string {
	encoded, _ := json.Marshal(value)
	return strings.ReplaceAll(string(encoded), "\x7f", `\u007f`)
}
