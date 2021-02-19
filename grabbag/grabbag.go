package grabbag

import (
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"

	// "github.com/bbkane/kvcrutch/sugarkane"

	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
)

// PretendToUse is an empty function to shut up the Go compiler while prototyping
func PretendToUse(args ...interface{}) {

}

// ValidateDirectory expands a directory and checks that it exists
// it returns the full path to the directory on success
// ValidateDirectory("~/foo") -> ("/home/bbkane/foo", nil)
func ValidateDirectory(dir string) (string, error) {
	dirPath, err := homedir.Expand(dir)
	if err != nil {
		return "", errors.WithStack(err)
	}
	info, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		return "", errors.Wrapf(err, "Directory does not exist: %v\n", dirPath)
	}
	if err != nil {
		return "", errors.Wrapf(err, "Directory error: %v\n", dirPath)

	}
	if !info.IsDir() {
		return "", errors.Errorf("Directory is a file, not a directory: %#v\n", dirPath)
	}
	return dirPath, nil
}

// EditFile writes a file if it does not exist, then opens the file to edit
func EditFile(defaultContent []byte, filePath string, editorPath string) error {

	filePath, err := homedir.Expand(filePath)
	if err != nil {
		err := errors.WithStack(err)
		return err
	}

	_, statErr := os.Stat(filePath)

	if os.IsNotExist(statErr) {
		writeErr := ioutil.WriteFile(filePath, defaultContent, 0644)
		writeErr = errors.Wrap(writeErr, "can't write config")
		if writeErr != nil {
			return writeErr
		}

	} else if statErr != nil {
		statErr = errors.Wrap(statErr, "can't stat config")
		return statErr
	}

	if editorPath == "" {
		editorPath = os.Getenv("EDITOR")
	}
	if editorPath == "" {
		if runtime.GOOS == "windows" {
			editorPath = "notepad"
		} else if runtime.GOOS == "darwin" {
			editorPath = "open"
		} else if runtime.GOOS == "linux" {
			editorPath = "xdg-open"
		} else {
			editorPath = "vim"
		}
	}
	executable, err := exec.LookPath(editorPath)
	if err != nil {
		err = errors.Wrap(err, "can't find editor")
		return err
	}

	cmd := exec.Command(executable, filePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}
