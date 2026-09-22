package domain

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ByteSize is a positive resource size in bytes.
type ByteSize int64

// GiB is the number of bytes in one gibibyte.
const GiB int64 = 1 << 30

var sizePattern = regexp.MustCompile(`^[1-9][0-9]*GiB$`)

// NewByteSize validates a positive byte count.
func NewByteSize(value int64) (ByteSize, error) {
	if value <= 0 {
		return 0, ErrInvalidByteSize
	}

	return ByteSize(value), nil
}

// ParseByteSize reads the whole-GiB notation used by configuration.
func ParseByteSize(value string) (ByteSize, error) {
	if !sizePattern.MatchString(value) {
		return 0, ErrInvalidSizeFormat
	}

	amount, err := strconv.ParseInt(strings.TrimSuffix(value, "GiB"), 10, 64)
	if err != nil || amount > math.MaxInt64/GiB {
		return 0, ErrSizeOverflow
	}

	return NewByteSize(amount * GiB)
}

// GiB formats a size as a positive whole number of gibibytes.
func (size ByteSize) GiB() (string, error) {
	if size <= 0 || int64(size)%GiB != 0 {
		return "", ErrNotWholeGiB
	}

	return strconv.FormatInt(int64(size)/GiB, 10) + "GiB", nil
}

// MarshalText writes the configuration representation.
func (size ByteSize) MarshalText() ([]byte, error) {
	value, err := size.GiB()
	return []byte(value), err
}

// UnmarshalText reads the configuration representation.
func (size *ByteSize) UnmarshalText(data []byte) error {
	parsed, err := ParseByteSize(string(data))
	if err != nil {
		return err
	}

	*size = parsed
	return nil
}

// MarshalJSON writes the whole-GiB representation as a JSON string.
func (size ByteSize) MarshalJSON() ([]byte, error) {
	value, err := size.GiB()
	if err != nil {
		return nil, err
	}

	return json.Marshal(value)
}

// UnmarshalJSON requires a JSON string containing a whole-GiB size.
func (size *ByteSize) UnmarshalJSON(data []byte) error {
	var value string

	if err := json.Unmarshal(data, &value); err != nil {
		return ErrSizeNotString
	}

	return size.UnmarshalText([]byte(value))
}
