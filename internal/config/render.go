package config

import (
	"encoding/json"
	"fmt"
	"html"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// RenderExample renders the public defaults from the same model used by parsing.
func RenderExample() ([]byte, error) { return Render(Default()) }

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
		item := model.Type().Field(index)
		value := model.Field(index)
		if value.Kind() == reflect.Struct || value.Kind() == reflect.Map || isTableArray(value) && value.Len() > 0 {
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
		item := model.Type().Field(index)
		value := model.Field(index)
		name := joinField(prefix, item.Tag.Get("toml"))
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

func sortedMapKeys(value reflect.Value) []reflect.Value {
	keys := value.MapKeys()
	sort.Slice(keys, func(a, b int) bool { return keys[a].String() < keys[b].String() })
	return keys
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
	// JSON leaves DEL literal, while TOML basic strings require it escaped.
	return strings.ReplaceAll(string(encoded), "\x7f", `\u007f`)
}

func typeName(item reflect.StructField) string {
	if item.Type == reflect.TypeFor[domain.Architecture]() {
		values := domain.Architectures()
		choices := make([]string, len(values))
		for index, value := range values {
			choices[index] = quoteTOMLString(string(value))
		}
		return strings.Join(choices, " or ")
	}
	if choices := item.Tag.Get("choices"); choices != "" {
		values := strings.Split(choices, ",")
		for index := range values {
			values[index] = quoteTOMLString(values[index])
		}
		return strings.Join(values, " or ")
	}
	return modelTypeName(item.Type)
}

func modelTypeName(model reflect.Type) string {
	if model == byteSizeType {
		return "string (GiB)"
	}
	switch model.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int:
		return "integer"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice:
		return "array[" + modelTypeName(model.Elem()) + "]"
	case reflect.Map:
		return "table[" + modelTypeName(model.Key()) + ", " + modelTypeName(model.Elem()) + "]"
	default:
		return model.Name()
	}
}

func escapeMarkdownCell(value string) string {
	return strings.NewReplacer("|", "&#124;", "\n", " ", "\r", " ").Replace(html.EscapeString(value))
}

func markdownCode(value string) string {
	value = escapeMarkdownCell(value)
	for _, character := range "`*_[]\\" {
		value = strings.ReplaceAll(value, string(character), fmt.Sprintf("&#%d;", character))
	}
	return "<code>" + value + "</code>"
}

// RenderReference returns Markdown field tables generated from types, tags and defaults.
func RenderReference() (string, error) {
	var output strings.Builder
	if err := referenceModel(&output, reflect.ValueOf(Default()), ""); err != nil {
		return "", err
	}
	return output.String(), nil
}

func referenceModel(output *strings.Builder, model reflect.Value, prefix string) error {
	if prefix == "" {
		output.WriteString("## Top-level fields\n\n")
	} else {
		output.WriteString("## `" + prefix + "`\n\n")
	}
	output.WriteString("| Field | Type | Default | Description |\n| --- | --- | --- | --- |\n")
	for index := range model.NumField() {
		item := model.Type().Field(index)
		value := model.Field(index)
		if value.Kind() == reflect.Struct || value.Kind() == reflect.Map || isTableArray(value) {
			continue
		}
		encoded, err := tomlLiteral(value)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "| %s | %s | %s | %s |\n", markdownCode(item.Tag.Get("toml")), markdownCode(typeName(item)), markdownCode(encoded), escapeMarkdownCell(item.Tag.Get("doc")))
	}
	output.WriteByte('\n')
	for index := range model.NumField() {
		item := model.Type().Field(index)
		value := model.Field(index)
		name := joinField(prefix, item.Tag.Get("toml"))
		switch {
		case value.Kind() == reflect.Struct:
			if err := referenceModel(output, value, name); err != nil {
				return err
			}
		case value.Kind() == reflect.Map:
			fmt.Fprintf(output, "## `%s`\n\n%s\n\nType: %s.\n\n| Default key | Default value |\n| --- | --- |\n", name, item.Tag.Get("doc"), markdownCode(typeName(item)))
			for _, key := range sortedMapKeys(value) {
				encoded, err := tomlLiteral(value.MapIndex(key))
				if err != nil {
					return err
				}
				fmt.Fprintf(output, "| %s | %s |\n", markdownCode(key.String()), markdownCode(encoded))
			}
			output.WriteByte('\n')
		case isTableArray(value):
			fmt.Fprintf(output, "## `%s`\n\n%s\n\n| Field | Type | Default per entry | Description |\n| --- | --- | --- | --- |\n", name, item.Tag.Get("doc"))
			entryType := value.Type().Elem()
			for field := range entryType.NumField() {
				entry := entryType.Field(field)
				defaultValue := "Required"
				if declared, exists := entry.Tag.Lookup("default"); exists {
					defaultValue = markdownCode(quoteTOMLString(declared))
				}
				fmt.Fprintf(output, "| %s | %s | %s | %s |\n", markdownCode(entry.Tag.Get("toml")), markdownCode(typeName(entry)), defaultValue, escapeMarkdownCell(entry.Tag.Get("doc")))
			}
			output.WriteString("\n### Default entries\n\n")
			if value.Len() == 0 {
				output.WriteString("Default: `[]`.\n\n")
				continue
			}
			columns := make([]string, entryType.NumField())
			separators := make([]string, len(columns))
			for field := range entryType.NumField() {
				columns[field] = markdownCode(entryType.Field(field).Tag.Get("toml"))
				separators[field] = "---"
			}
			output.WriteString("| " + strings.Join(columns, " | ") + " |\n| " + strings.Join(separators, " | ") + " |\n")
			for index := range value.Len() {
				for field := range entryType.NumField() {
					encoded, err := tomlLiteral(value.Index(index).Field(field))
					if err != nil {
						return err
					}
					columns[field] = markdownCode(encoded)
				}
				output.WriteString("| " + strings.Join(columns, " | ") + " |\n")
			}
			output.WriteByte('\n')
		}
	}
	return nil
}
