package generator

import (
	"bytes"
	"compress/gzip"
	"debug/buildinfo"
	"debug/elf"
	"errors"
	"fmt"
	"io"
)

// inspectBinary validates the executable before trusting its Go build metadata.
func inspectBinary(binary []byte, t target) (buildMetadata, error) {
	if err := validateBinary(binary, t.machine); err != nil {
		return buildMetadata{}, err
	}
	info, err := buildinfo.Read(bytes.NewReader(binary))
	if err != nil {
		return buildMetadata{}, fmt.Errorf("read Go build info: %w", err)
	}
	return metadataFromBuildInfo(info), nil
}

// validateBinary requires the target's 64-bit little-endian ELF with no runtime loader.
func validateBinary(binary []byte, machine elf.Machine) error {
	file, err := elf.NewFile(bytes.NewReader(binary))
	if err != nil {
		return fmt.Errorf("read ELF: %w", err)
	}
	defer func() { _ = file.Close() }()
	if file.Class != elf.ELFCLASS64 || file.Data != elf.ELFDATA2LSB || file.Machine != machine {
		return fmt.Errorf("ELF architecture: have %s/%s/%s, want ELFCLASS64/ELFDATA2LSB/%s", file.Class, file.Data, file.Machine, machine)
	}
	if file.Type != elf.ET_EXEC && file.Type != elf.ET_DYN {
		return fmt.Errorf("unsupported ELF type: %s", file.Type)
	}
	for _, program := range file.Progs {
		if program.Type == elf.PT_INTERP {
			return errors.New("guest agent requires a dynamic loader (PT_INTERP); a static executable is required")
		}
	}
	return nil
}

func gzipBinary(binary []byte) ([]byte, error) {
	var output bytes.Buffer
	writer, err := gzip.NewWriterLevel(&output, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	writer.OS = 255
	if _, err := writer.Write(binary); err != nil {
		return nil, errors.Join(err, writer.Close())
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func unpackArchive(archive []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	binary, readErr := io.ReadAll(reader)
	return binary, errors.Join(readErr, reader.Close())
}
