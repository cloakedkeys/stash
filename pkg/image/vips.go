package image

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/stashapp/stash/pkg/exec"
	"github.com/stashapp/stash/pkg/logger"
)

type vipsEncoder string

func (e *vipsEncoder) ImageThumbnail(image *bytes.Buffer, maxSize int) ([]byte, error) {
	args := []string{
		"thumbnail_source",
		"[descriptor=0]",
		".jpg[Q=70,strip]",
		fmt.Sprint(maxSize),
		"--size", "down",
	}
	data, err := e.run(args, image)

	return []byte(data), err
}

// ImageThumbnailPath generates a thumbnail from a file path instead of stdin.
// This is required for formats like AVIF that need random file access (seeking)
// which stdin cannot provide.
func (e *vipsEncoder) ImageThumbnailPath(path string, maxSize int) ([]byte, error) {
	return e.imageThumbnailPath(path, ".jpg[Q=70,strip]", maxSize)
}

func (e *vipsEncoder) imageThumbnailPath(path, output string, maxSize int) ([]byte, error) {
	// vips thumbnail syntax: thumbnail input output width [options]
	// Using a suffix as output writes to stdout
	args := []string{
		"thumbnail",
		path,
		output,
		fmt.Sprint(maxSize),
		"--size", "down",
	}

	cmd := exec.Command(string(*e), args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := exec.Start(cmd); err != nil {
		return nil, err
	}

	if err := cmd.Wait(); err != nil {
		logger.Errorf("image encoder error when running command <%s>: %s", strings.Join(cmd.Args, " "), stderr.String())
		return nil, err
	}

	return stdout.Bytes(), nil
}

func (e *vipsEncoder) run(args []string, stdin *bytes.Buffer) (string, error) {
	cmd := exec.Command(string(*e), args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = stdin

	if err := exec.Start(cmd); err != nil {
		return "", err
	}

	err := cmd.Wait()

	if err != nil {
		// error message should be in the stderr stream
		logger.Errorf("image encoder error when running command <%s>: %s", strings.Join(cmd.Args, " "), stderr.String())
		return stdout.String(), err
	}

	return stdout.String(), nil
}
