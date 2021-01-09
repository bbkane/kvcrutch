package grabbag

import (
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"

	"github.com/bbkane/kvcrutch/sugarkane"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
)

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

func EditConfig(defaultConfig []byte, configPath string, editor string) error {

	configPath, err := homedir.Expand(configPath)
	if err != nil {
		err := errors.WithStack(err)
		sugarkane.Printw(os.Stderr,
			"can't expand path",
			"configPath", configPath,
			"err", err,
		)
	}

	stat, statErr := os.Stat(configPath)

	if os.IsNotExist(statErr) {
		writeErr := ioutil.WriteFile(configPath, defaultConfig, 0644)
		if writeErr != nil {
			sugarkane.Printw(os.Stderr,
				"can't write new config",
				"stat", stat,
				"statErr", statErr,
				"writeErr", writeErr,
			)
			return writeErr
		}
		sugarkane.Printw(os.Stdout,
			"wrote default config",
			"configPath", configPath,
		)
	} else if statErr != nil {
		sugarkane.Printw(os.Stderr,
			"can't stat config",
			"stat", stat,
			"statErr", statErr,
		)
		return statErr
	}

	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else if runtime.GOOS == "darwin" {
			editor = "open"
		} else if runtime.GOOS == "linux" {
			editor = "xdg-open"
		} else {
			editor = "vim"
		}
	}
	executable, err := exec.LookPath(editor)
	if err != nil {
		sugarkane.Printw(os.Stderr,
			"can't find editor",
			"err", err,
		)
		return err
	}

	sugarkane.Printw(os.Stderr,
		"Opening config",
		"editor", executable,
		"configPath", configPath,
	)

	cmd := exec.Command(executable, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		sugarkane.Printw(os.Stderr,
			"editor cmd error",
			"err", err,
		)
		return err
	}

	return nil
}
