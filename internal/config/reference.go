package config

import (
	"fmt"
	"html"
	"reflect"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/domain"
)

// RenderReference returns Markdown field tables generated from types, tags and defaults.
func RenderReference() (string, error) {
	var output strings.Builder

	if err := referenceModel(&output, reflect.ValueOf(Default()), ""); err != nil {
		return "", err
	}

	return output.String(), nil
}

func typeName(item reflect.StructField) string {
	if item.Type == reflect.TypeFor[domain.Architecture]() {
		var (
			values  = domain.Architectures()
			choices = make([]string, len(values))
		)

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

func referenceModel(output *strings.Builder, model reflect.Value, prefix string) error {
	if prefix == "" {
		output.WriteString("## Top-level fields\n\n")
	} else {
		output.WriteString("## `" + prefix + "`\n\n")
	}

	output.WriteString("| Field | Type | Default | Description |\n| --- | --- | --- | --- |\n")

	for index := range model.NumField() {
		var (
			item  = model.Type().Field(index)
			value = model.Field(index)
		)

		if value.Kind() == reflect.Struct || value.Kind() == reflect.Map || isTableArray(value) {
			continue
		}

		encoded, err := tomlLiteral(value)
		if err != nil {
			return err
		}

		referenceRow(output, item, markdownCode(encoded))
	}

	output.WriteByte('\n')

	for index := range model.NumField() {
		var (
			item  = model.Type().Field(index)
			value = model.Field(index)
			name  = joinField(prefix, item.Tag.Get("toml"))
			err   error
		)

		switch {
		case value.Kind() == reflect.Struct:
			err = referenceModel(output, value, name)
		case value.Kind() == reflect.Map:
			err = referenceMap(output, item, value, name)
		case isTableArray(value):
			err = referenceEntries(output, item, value, name)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func referenceRow(output *strings.Builder, item reflect.StructField, defaultValue string) {
	fmt.Fprintf(
		output,
		"| %s | %s | %s | %s |\n",
		markdownCode(item.Tag.Get("toml")),
		markdownCode(typeName(item)),
		defaultValue,
		escapeMarkdownCell(item.Tag.Get("doc")),
	)
}

func referenceMap(output *strings.Builder, item reflect.StructField, value reflect.Value, name string) error {
	fmt.Fprintf(
		output,
		"## `%s`\n\n%s\n\nType: %s.\n\n| Default key | Default value |\n| --- | --- |\n",
		name,
		item.Tag.Get("doc"),
		markdownCode(typeName(item)),
	)

	for _, key := range sortedMapKeys(value) {
		encoded, err := tomlLiteral(value.MapIndex(key))
		if err != nil {
			return err
		}

		fmt.Fprintf(output, "| %s | %s |\n", markdownCode(key.String()), markdownCode(encoded))
	}

	output.WriteByte('\n')
	return nil
}

func referenceEntries(output *strings.Builder, item reflect.StructField, value reflect.Value, name string) error {
	fmt.Fprintf(
		output,
		"## `%s`\n\n%s\n\n| Field | Type | Default per entry | Description |\n| --- | --- | --- | --- |\n",
		name,
		item.Tag.Get("doc"),
	)

	entryType := value.Type().Elem()

	for index := range entryType.NumField() {
		var (
			entry        = entryType.Field(index)
			defaultValue = "Required"
		)

		if declared, exists := entry.Tag.Lookup("default"); exists {
			defaultValue = markdownCode(quoteTOMLString(declared))
		}

		referenceRow(output, entry, defaultValue)
	}

	output.WriteString("\n### Default entries\n\n")
	if value.Len() == 0 {
		output.WriteString("Default: `[]`.\n\n")
		return nil
	}

	return referenceDefaults(output, value)
}

func referenceDefaults(output *strings.Builder, entries reflect.Value) error {
	var (
		entryType  = entries.Type().Elem()
		columns    = make([]string, entryType.NumField())
		separators = make([]string, len(columns))
	)

	for field := range entryType.NumField() {
		columns[field] = markdownCode(entryType.Field(field).Tag.Get("toml"))
		separators[field] = "---"
	}

	output.WriteString("| " + strings.Join(columns, " | ") + " |\n")
	output.WriteString("| " + strings.Join(separators, " | ") + " |\n")

	for index := range entries.Len() {
		for field := range entryType.NumField() {
			encoded, err := tomlLiteral(entries.Index(index).Field(field))
			if err != nil {
				return err
			}

			columns[field] = markdownCode(encoded)
		}

		output.WriteString("| " + strings.Join(columns, " | ") + " |\n")
	}

	output.WriteByte('\n')
	return nil
}
