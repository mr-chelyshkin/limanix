package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/mr-chelyshkin/limanix/internal/domain"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
	"golang.org/x/sys/unix"
)

const schemaVersion = 1

// identityRecord persists immutable ownership separately from operation progress.
type identityRecord struct {
	SchemaVersion int `json:"schema_version"`
	domain.Identity
}

// runtimeRecord is the mutable, versioned state of one VM operation.
type runtimeRecord struct {
	SchemaVersion int           `json:"schema_version"`
	Status        domain.Status `json:"status"`
	Generation    string        `json:"generation"`
	Error         *string       `json:"error"`
}

func decodeIdentityRecord(record identityRecord, name domain.VMName) (domain.Identity, error) {
	if record.SchemaVersion != schemaVersion {
		return domain.Identity{}, ErrUnsupportedSchema
	}

	if record.Name != name {
		return domain.Identity{}, ErrIdentityName
	}

	return record.Normalized()
}

func validateRuntime(record runtimeRecord) error {
	if record.SchemaVersion != schemaVersion {
		return ErrUnsupportedSchema
	}

	if !domain.ValidIdentifier(record.Generation) {
		return ErrInvalidGeneration
	}

	switch record.Status {
	case domain.Creating, domain.Ready, domain.Updating, domain.Failed, domain.Deleting:
		return nil
	case domain.Interrupted:
		return ErrComputedStatus
	default:
		return ErrInvalidStatus
	}
}

func writeRecord(path string, record any) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}

	return filesystem.WriteFileAtomic(path, append(data, '\n'), 0o600)
}

// readRecord requires all model fields, refuses extras, and never follows final symlinks.
func readRecord(path string, record any) error {
	file, err := filesystem.OpenRegular(path, unix.O_RDONLY, 0)
	if err != nil {
		return err
	}

	data, readErr := io.ReadAll(file)
	if err = errors.Join(readErr, file.Close()); err != nil {
		return err
	}

	if !utf8.Valid(data) {
		return ErrRecordEncoding
	}

	var fields map[string]json.RawMessage

	if err = json.Unmarshal(data, &fields); err != nil {
		return err
	}

	expected := jsonFieldNames(reflect.TypeOf(record).Elem())
	if len(fields) != len(expected) {
		return ErrRecordFields
	}

	for _, name := range expected {
		if _, exists := fields[name]; !exists {
			return ErrRecordFields
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(record)
}

func jsonFieldNames(model reflect.Type) []string {
	var result []string

	for index := range model.NumField() {
		field := model.Field(index)
		if field.Anonymous {
			result = append(result, jsonFieldNames(field.Type)...)
			continue
		}

		result = append(result, strings.Split(field.Tag.Get("json"), ",")[0])
	}

	return result
}
