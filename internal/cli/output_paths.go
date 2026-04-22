package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gvkhna/clawchrome-cli/internal/client"
)

func resolveOutputFilePath(command string, pathArg string, defaultPattern string) (string, error) {
	if pathArg == "" {
		return uniqueTempOutputPath(command, defaultPattern)
	}

	path, err := filepath.Abs(pathArg)
	if err != nil {
		return "", validationError(command, fmt.Sprintf("Invalid output path %q: %v", pathArg, err))
	}
	path = filepath.Clean(path)

	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return "", validationError(command, "Output path is a directory: "+path)
		}
		if !info.Mode().IsRegular() {
			return "", validationError(command, "Output path is not a regular file: "+path)
		}
		if err := validateWritableFile(path); err != nil {
			return "", validationError(command, fmt.Sprintf("Output file is not writable: %s (%v)", path, err))
		}
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", validationError(command, fmt.Sprintf("Cannot inspect output path: %s (%v)", path, err))
	}

	dir := filepath.Dir(path)
	if err := validateWritableDirectory(command, "Output directory", dir, path); err != nil {
		return "", err
	}
	return path, nil
}

func uniqueTempOutputPath(command string, pattern string) (string, error) {
	dir := os.TempDir()
	if dir == "" {
		return "", validationError(command, "Cannot determine temporary output directory")
	}

	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", validationError(command, fmt.Sprintf("Invalid temporary output directory %q: %v", os.TempDir(), err))
	}
	dir = filepath.Clean(dir)

	if err := validateWritableDirectory(command, "Temporary output directory", dir, ""); err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", validationError(command, fmt.Sprintf("Could not create temporary output path in %s (%v)", dir, err))
	}
	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return "", validationError(command, fmt.Sprintf("Could not close temporary output path: %s (%v)", path, err))
	}
	if err := os.Remove(path); err != nil {
		return "", validationError(command, fmt.Sprintf("Could not reserve temporary output path: %s (%v)", path, err))
	}
	return path, nil
}

func validateWritableDirectory(command string, label string, dir string, outputPath string) error {
	context := ""
	if outputPath != "" {
		context = " (for output path: " + outputPath + ")"
	}

	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return validationError(command, fmt.Sprintf("%s does not exist: %s%s", label, dir, context))
	}
	if err != nil {
		return validationError(command, fmt.Sprintf("Cannot inspect %s: %s%s (%v)", lowerFirst(label), dir, context, err))
	}
	if !info.IsDir() {
		return validationError(command, fmt.Sprintf("%s is not a directory: %s%s", label, dir, context))
	}
	if err := probeWritableDirectory(dir); err != nil {
		return validationError(command, fmt.Sprintf("%s is not writable: %s%s (%v)", label, dir, context, err))
	}
	return nil
}

func probeWritableDirectory(dir string) error {
	tmp, err := os.CreateTemp(dir, ".clawchrome-cli-write-test-*")
	if err != nil {
		return err
	}
	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return os.Remove(path)
}

func validateWritableFile(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

func outputPathOperationError(action string, path string, err error) error {
	if cdpErr, ok := err.(*client.CdpError); ok {
		suggestions := append([]string(nil), cdpErr.Suggestions...)
		return client.WrapError(fmt.Sprintf("Failed to %s at %s: %s", action, path, cdpErr.Message), cdpErr.Code, suggestions...)
	}
	return client.WrapError(fmt.Sprintf("Failed to %s at %s: %v", action, path, err), client.ErrUnknown)
}

func lowerFirst(text string) string {
	if text == "" {
		return text
	}
	if text[0] >= 'A' && text[0] <= 'Z' {
		return string(text[0]+('a'-'A')) + text[1:]
	}
	return text
}
