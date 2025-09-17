package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// Create creates a tar archive from the contents of srcDir and writes it to dstPath.
// If compress is true, the tar archive will be gzip-compressed.
func Create(srcDir, dstPath string, compress bool) error {
	f, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create destination file %q: %w", dstPath, err)
	}
	defer f.Close()

	var w io.WriteCloser = f
	if compress {
		gzipWriter := gzip.NewWriter(f)
		defer gzipWriter.Close()

		w = gzipWriter
	}

	tarWriter := tar.NewWriter(w)
	defer tarWriter.Close()

	return filepath.Walk(srcDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && path == srcDir {
			// don't add the root itself
			return nil
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return fmt.Errorf("create tar header for %q: %w", path, err)
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("compute relative path for %q: %w", path, err)
		}
		header.Name = filepath.ToSlash(relPath)

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("write tar header for %q: %w", path, err)
		}

		// if it's a regular file, copy its contents
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("open file %q: %w", path, err)
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return fmt.Errorf("copy file %q to tar: %w", path, err)
			}

			log.Debug().Msgf("added file to archive: %s", header.Name)
		}

		return nil
	})
}
