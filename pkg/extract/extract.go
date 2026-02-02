package extract

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

type Options struct {
	StripLevels int

	Perm *os.FileMode
	UID  *int
	GID  *int
}

type Option func(o *Options)

func StripLevels(levels int) Option {
	return func(o *Options) {
		o.StripLevels = levels
	}
}

func Extract(origReader io.Reader, destFolder string, options ...Option) error {
	extractOptions := &Options{}
	for _, o := range options {
		o(extractOptions)
	}

	// read ahead
	bufioReader := bufio.NewReaderSize(origReader, 1024*1024)
	testBytes, err := bufioReader.Peek(2) // read 2 bytes
	if err != nil {
		return err
	}

	// is gzipped?
	var reader io.Reader
	if testBytes[0] == 31 && testBytes[1] == 139 {
		gzipReader, err := gzip.NewReader(bufioReader)
		if err != nil {
			return fmt.Errorf("error decompressing: %v", err)
		}
		defer func() { _ = gzipReader.Close() }()

		reader = gzipReader
	} else {
		reader = bufioReader
	}

	tarReader := tar.NewReader(reader)
	for {
		shouldContinue, err := extractNext(tarReader, destFolder, extractOptions)
		if err != nil {
			return fmt.Errorf("decompress: %w", err)
		} else if !shouldContinue {
			return nil
		}
	}
}

func extractNext(tarReader *tar.Reader, destFolder string, options *Options) (bool, error) {
	header, err := tarReader.Next()
	if err != nil {
		if !errors.Is(err, io.EOF) {
			return false, fmt.Errorf("tar reader next: %w", err)
		}

		return false, nil
	}

	relativePath := getRelativeFromFullPath("/"+header.Name, "")
	if options.StripLevels > 0 {
		for i := 0; i < options.StripLevels; i++ {
			relativePath = strings.TrimPrefix(relativePath, "/")
			index := strings.Index(relativePath, "/")
			if index == -1 {
				break
			}

			relativePath = relativePath[index+1:]
		}

		relativePath = "/" + relativePath
	}
	outFileName := path.Join(destFolder, relativePath)
	baseName := path.Dir(outFileName)

	dirPerm := os.ModePerm
	if options.Perm != nil {
		dirPerm = *options.Perm
	}

	// Check if newer file is there and then don't override?
	if err := os.MkdirAll(baseName, dirPerm); err != nil {
		return false, err
	}

	// whats the file perm?
	filePerm := os.FileMode(0644)
	if options.Perm != nil {
		filePerm = *options.Perm
	}

	// Is dir?
	switch header.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(outFileName, dirPerm); err != nil {
			return false, err
		}

		return true, nil
	case tar.TypeSymlink:
		err := os.Symlink(header.Linkname, outFileName)
		if err != nil {
			return false, err
		}

		return true, nil
	case tar.TypeLink:
		err := os.Link(header.Linkname, outFileName)
		if err != nil {
			return false, err
		}

		return true, nil
	}

	// Create / Override file
	outFile, err := os.OpenFile(outFileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, filePerm)
	if err != nil {
		// Try again after 5 seconds
		time.Sleep(time.Second * 5)
		outFile, err = os.OpenFile(outFileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, filePerm)
		if err != nil {
			return false, fmt.Errorf("create %s: %w", outFileName, err)
		}
	}
	defer func() { _ = outFile.Close() }()

	if _, err := io.Copy(outFile, tarReader); err != nil {
		return false, fmt.Errorf("io copy tar reader %s: %w", outFileName, err)
	}
	if err := outFile.Close(); err != nil {
		return false, fmt.Errorf("out file close %s: %w", outFileName, err)
	}

	// Set permissions
	if options.Perm == nil {
		_ = os.Chmod(outFileName, header.FileInfo().Mode()|0600)
	}

	// Set mod time from tar header
	_ = os.Chtimes(outFileName, time.Now(), header.FileInfo().ModTime())

	return true, nil
}

func getRelativeFromFullPath(fullpath string, prefix string) string {
	return strings.TrimPrefix(strings.ReplaceAll(strings.ReplaceAll(fullpath[len(prefix):], "\\", "/"), "//", "/"), ".")
}
