package modulegen

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/mr-chelyshkin/limanix/internal/nixos/catalog"
)

// pack strips GitHub's enclosing directory and retains only the distributable catalog.
func pack(ctx context.Context, source io.Reader, version string) (data []byte, failure error) {
	compressed, err := gzip.NewReader(source)
	if err != nil {
		return nil, fmt.Errorf("read source archive: %w", err)
	}
	defer func() {
		failure = errors.Join(failure, compressed.Close())
	}()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	if err = writer.SetComment(catalog.Repository); err != nil {
		return nil, err
	}

	reader := tar.NewReader(compressed)
	var root string

	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}

		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read source archive entry: %w", err)
		}

		// Git archives carry their commit comment in a global PAX header, not a file.
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}

		name := strings.TrimSuffix(header.Name, "/")
		if !fs.ValidPath(name) || name == "." || strings.Contains(name, `\`) {
			return nil, fmt.Errorf("%w: unsafe source path %q", catalog.ErrArchive, header.Name)
		}

		prefix, relative, _ := strings.Cut(name, "/")
		if root == "" {
			root = prefix
		}
		if root != prefix {
			return nil, fmt.Errorf("%w: multiple source roots", catalog.ErrArchive)
		}

		if relative != "LICENSE" && !strings.HasPrefix(relative, "modules/") {
			continue
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("%w: unsupported source file %q", catalog.ErrArchive, header.Name)
		}

		entry := &zip.FileHeader{Name: relative, Method: zip.Deflate}
		entry.SetMode(header.FileInfo().Mode().Perm())
		output, err := writer.CreateHeader(entry)
		if err != nil {
			return nil, err
		}
		if _, err = io.Copy(output, reader); err != nil {
			return nil, err
		}
	}

	// Drain the gzip trailer as well: tar's end marker alone does not verify it.
	if _, err = io.Copy(io.Discard, compressed); err != nil {
		return nil, fmt.Errorf("read source archive trailer: %w", err)
	}

	output, err := writer.Create("version")
	if err != nil {
		return nil, err
	}
	if _, err = io.WriteString(output, version+"\n"); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
