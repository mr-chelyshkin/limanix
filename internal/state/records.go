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

type identityRecord struct {
	SchemaVersion int `json:"schema_version"`
	Identity
}

type runtimeRecord struct {
	SchemaVersion int     `json:"schema_version"`
	Status        Status  `json:"status"`
	Generation    string  `json:"generation"`
	Error         *string `json:"error"`
}

func decodeIdentityRecord(record identityRecord, name domain.VMName) (Identity, error) {
	if record.SchemaVersion != schemaVersion {
		return Identity{}, errors.New("unsupported state schema")
	}
	if record.Name != name {
		return Identity{}, errors.New("identity name differs from its directory")
	}
	return validatedIdentity(record.Identity)
}

func validateRuntime(record runtimeRecord) error {
	if record.SchemaVersion != schemaVersion {
		return errors.New("unsupported state schema")
	}
	if !identifierPattern.MatchString(record.Generation) {
		return errors.New("invalid generation")
	}

	switch record.Status {
	case Creating, Ready, Updating, Error, Deleting:
		return nil
	case Interrupted:
		return errors.New("interrupted is a computed listing status")
	default:
		return errors.New("invalid lifecycle state")
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
		return errors.New("record is not valid UTF-8")
	}

	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return err
	}

	expected := jsonFieldNames(reflect.TypeOf(record).Elem())
	if len(fields) != len(expected) {
		return errors.New("unexpected record fields")
	}
	for _, name := range expected {
		if _, exists := fields[name]; !exists {
			return errors.New("unexpected record fields")
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
