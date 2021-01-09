//go:generate go run ./generate_static_data.go
package main

import (
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/bbkane/kvcrutch/kvcrutch"
	"github.com/bbkane/kvcrutch/static"
	"github.com/bbkane/kvcrutch/sugarkane"

	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/yaml.v2"
)

// These will be overwritten by goreleaser
var version = "devVersion"
var commit = "devCommit"
var date = "devDate"
var builtBy = "devBuiltBy"

type config struct {
	Version                     string
	LumberjackLogger            *lumberjack.Logger                      `yaml:"lumberjacklogger"`
	VaultName                   string                                  `yaml:"vault_name"`
	CertificateCreateParameters kvcrutch.CfgCertificateCreateParameters `yaml:"certificate_create_parameters"`
}

func parseConfig(configBytes []byte) (*lumberjack.Logger, string, kvcrutch.CfgCertificateCreateParameters, error) {

	cfg := config{}
	err := yaml.UnmarshalStrict(configBytes, &cfg)
	if err != nil {
		// not ok to get invalid YAML
		return nil, "", kvcrutch.CfgCertificateCreateParameters{}, errors.WithStack(err)
	}

	var lumberjackLogger *lumberjack.Logger = nil

	// we can get a valid config with a nil logger
	if cfg.LumberjackLogger != nil {
		// Note that if the directories to here don't exist, lumberjack will
		// make them
		f, err := homedir.Expand(cfg.LumberjackLogger.Filename)
		if err != nil {
			return nil, "", kvcrutch.CfgCertificateCreateParameters{}, errors.WithStack(err)
		}
		cfg.LumberjackLogger.Filename = f
		lumberjackLogger = cfg.LumberjackLogger
	}

	return lumberjackLogger, cfg.VaultName, cfg.CertificateCreateParameters, nil
}

func pretendToUse(args ...interface{}) {

}

// validateDirectory expands a directory and checks that it exists
// it returns the full path to the directory on success
// validateDirectory("~/foo") -> ("/home/bbkane/foo", nil)
func validateDirectory(dir string) (string, error) {
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

func editConfig(defaultConfig []byte, configPath string, editor string) error {

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

func run() error {

	// parse the CLI args
	app := kingpin.New("kvcrutch", "Lean on me when `az keyvault` isn't quite as useful as needed").UsageTemplate(kingpin.DefaultUsageTemplate)
	app.HelpFlag.Short('h')
	defaultConfigPath := "~/.config/kvcrutch.yaml"
	appConfigPathFlag := app.Flag("config-path", "config filepath").Short('c').Default(defaultConfigPath).String()
	appVaultNameFlag := app.Flag("vault-name", "Key Vault Name").Short('v').String()
	appTimeout := app.Flag("timeout", "limit keyvault operations when this expires. See https://golang.org/pkg/time/#ParseDuration for formatting details").Default("30s").String()

	configCmd := app.Command("config", "config commands")
	configCmdEditCmd := configCmd.Command("edit", "Edit or create configuration file. Uses $EDITOR as a fallback")
	configCmdEditCmdEditorFlag := configCmdEditCmd.Flag("editor", "path to editor").Short('e').String()

	certificateCmd := app.Command("certificate", "work with certificates")
	certificateCreateCmd := certificateCmd.Command("create", "create a certificate")
	certificateCreateCmdIDFlag := certificateCreateCmd.Flag("id", "certificate id in keyvault").Short('i').Required().String()
	certificateCreateCmdSubjectFlag := certificateCreateCmd.Flag("subject", "Certificate subject. Example: CN=example.com").String()
	certificateCreateCmdSANsFlag := certificateCreateCmd.Flag("san", "subject alternative DNS name").Strings()
	certificateCreateCmdTagsFlag := certificateCreateCmd.Flag("tag", "tags to add in key=value form").Short('t').Strings()
	certificateCreateCmdValidityInMonthsFlag := certificateCreateCmd.Flag("validity", "validity in months").Int32()
	certificateCreateCmdEnabledFlag := certificateCreateCmd.Flag("enabled", "enable certificate on creation").Short('e').Bool()
	certificateCreateCmdNewVersionOkFlag := certificateCreateCmd.Flag("new-version-ok", "Confirm it's ok to create a new version of a certificate").Short('n').Bool()
	certificateCreateCmdSkipConfirmationFlag := certificateCreateCmd.Flag("skip-confirmation", "create cert without prompting for confirmation").Bool()

	versionCmd := app.Command("version", "print kvcrutch build and version information")

	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	// work with commands that don't have dependencies (version, editConfig)
	configPath, err := homedir.Expand(*appConfigPathFlag)
	if err != nil {
		err = errors.WithStack(err)
		sugarkane.Printw(os.Stderr,
			"ERROR: config error",
			"err", err,
		)
		return err
	}

	if cmd == configCmdEditCmd.FullCommand() {
		configFile := "kvcrutch.yaml"
		fp, err := static.Static.Open(configFile)
		if err != nil {
			err = errors.Errorf("Can't open file: %#v\n", configFile)
			sugarkane.Printw(os.Stderr,
				"ERROR: can't open file",
				"file", configFile,
			)
			return err
		}
		configBytes, err := ioutil.ReadAll(fp)
		if err != nil {
			err = errors.Errorf("Can't read file: %#v\n", configBytes)
			sugarkane.Printw(os.Stderr,
				"ERROR: can't read file",
				"file", configFile,
			)
			return err
		}

		return editConfig(configBytes, *appConfigPathFlag, *configCmdEditCmdEditorFlag)
	}
	if cmd == versionCmd.FullCommand() {
		sugarkane.Printw(os.Stdout,
			"INFO: Version and build information",
			"builtBy", builtBy,
			"commit", commit,
			"date", date,
			"version", version,
		)
		return nil
	}

	// get a config
	configBytes, cfgLoadErr := ioutil.ReadFile(configPath)
	if cfgLoadErr != nil {
		if cfgLoadErr != nil {
			sugarkane.Printw(os.Stderr,
				"ERROR: Config error - try `config edit`",
				"cfgLoadErr", cfgLoadErr,
				"cfgLoadErrMsg", cfgLoadErr.Error(),
			)
			return cfgLoadErr
		}
	}

	lumberjackLogger, cfgVaultName, cfgCertCreateParams, cfgParseErr := parseConfig(configBytes)
	if cfgParseErr != nil {
		sugarkane.Printw(os.Stderr,
			"ERROR: Can't parse config",
			"err", cfgParseErr,
		)
		return cfgParseErr
	}

	// get a logger
	sk := sugarkane.NewSugarKane(lumberjackLogger, os.Stderr, os.Stdout, zap.DebugLevel, version)
	defer sk.Sync()
	sk.LogOnPanic()

	// get a keyvault client
	kvClient, err := kvcrutch.PrepareKV(sk)
	if err != nil {
		err := errors.WithStack(err)
		return err
	}

	// get the vaultURL
	vaultName := cfgVaultName
	if *appVaultNameFlag != "" {
		vaultName = *appVaultNameFlag
	}
	vaultURL := "https://" + vaultName + ".vault.azure.net"

	// get a timeout
	timeout, err := time.ParseDuration(*appTimeout)
	if err != nil {
		err := errors.WithStack(err)
		sk.Errorw(
			"can't parse app timeout",
			"err", err,
		)
	}

	// dispatch commands that use dependencies
	switch cmd {
	case certificateCreateCmd.FullCommand():
		flagTagsMap, err := kvcrutch.ParseTags(*certificateCreateCmdTagsFlag)
		if err != nil {
			err := errors.WithStack(err)
			sk.Errorw(
				"flag parsing error",
				"err", err,
			)
		}
		flagCertCreateParams := kvcrutch.FlagCertificateCreateParameters{
			Subject:          *certificateCreateCmdSubjectFlag,
			Sans:             *certificateCreateCmdSANsFlag,
			Tags:             flagTagsMap,
			ValidityInMonths: *certificateCreateCmdValidityInMonthsFlag,
			Enabled:          *certificateCreateCmdEnabledFlag,
		}
		return kvcrutch.CertificateCreate(
			sk,
			cfgCertCreateParams,
			*certificateCreateCmdIDFlag,
			flagCertCreateParams,
			*certificateCreateCmdNewVersionOkFlag,
			kvClient,
			vaultURL,
			*certificateCreateCmdSkipConfirmationFlag,
			timeout,
		)
	default:
		err = errors.Errorf("Unknown command: %#v\n", cmd)
		sk.Errorw(
			"Unknown command",
			"cmd", cmd,
			"err", err,
		)
		return err
	}
}

func main() {
	err := run()
	if err != nil {
		os.Exit(1)
	}
}
