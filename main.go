package main

import (
	"crypto/tls"
	_ "embed"
	"io/ioutil"
	"net"
	"os"
	"time"

	"github.com/bbkane/glib"
	kvcrutch "github.com/bbkane/kvcrutch/lib"
	"github.com/bbkane/sugarkane"

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

//go:embed embedded/kvcrutch.yaml
var embeddedConfig []byte

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

func run() error {

	// parse the CLI args
	app := kingpin.New("kvcrutch", "Augment `az keyvault`. See https://github.com/bbkane/kvcrutch for example usage").UsageTemplate(kingpin.DefaultUsageTemplate)
	app.HelpFlag.Short('h')
	defaultConfigPath := "~/.config/kvcrutch.yaml"
	appConfigPathFlag := app.Flag("config-path", "Config filepath. Example: ./kvcrutch.yaml").Short('c').Default(defaultConfigPath).String()
	appVaultNameFlag := app.Flag("vault-name", "Key Vault Name. Example: my-keyvault").Short('v').String()
	appTimeout := app.Flag("timeout", "Limit keyvault operations when this expires. See https://golang.org/pkg/time/#ParseDuration for formatting details. Example: 1m").Default("30s").String()

	configCmd := app.Command("config", "Config commands")
	configCmdEditCmd := configCmd.Command("edit", "Edit or create configuration file. Uses $EDITOR as a fallback")
	configCmdEditCmdEditorFlag := configCmdEditCmd.Flag("editor", "Path to editor").Short('e').String()

	certificateCmd := app.Command("certificate", "Work with certificates")

	certificateCreateCmd := certificateCmd.Command("create", "Create a certificate")
	certificateCreateCmdNameFlag := certificateCreateCmd.Flag("name", "certificate name in keyvault. Example: my-cert").Short('n').Required().String()
	certificateCreateCmdSubjectFlag := certificateCreateCmd.Flag("subject", "Certificate subject. Example: CN=example.com").String()
	certificateCreateCmdSANsFlag := certificateCreateCmd.Flag("san", "DNS Subject Alternative Name. Example: www.bbkane.com").Strings()
	certificateCreateCmdTagsFlag := certificateCreateCmd.Flag("tag", "Tags to add in key=value form. Example: mykey=myvalue").Short('t').Strings()
	certificateCreateCmdValidityInMonthsFlag := certificateCreateCmd.Flag("validity", "Validity in months. Example: 6").Int32()
	certificateCreateCmdEnabledFlag := certificateCreateCmd.Flag("enabled", "Enable certificate on creation").Short('e').Bool()
	certificateCreateCmdIssuerNameFlag := certificateCreateCmd.Flag("issuer-name", "CA Issuer name. Example: Self").String()
	certificateCreateCmdNewVersionOkFlag := certificateCreateCmd.Flag("new-version-ok", "Confirm it's ok to create a new version of a certificate").Bool()
	certificateCreateCmdSkipConfirmationFlag := certificateCreateCmd.Flag("skip-confirmation", "Create cert without prompting for confirmation").Bool()

	certificateListCmd := certificateCmd.Command("list", "List all certificates in a keyvault")

	certificateNewVersionCmd := certificateCmd.Command("new-version", "Create a new version of an existing certificate")
	certificateNewVersionCmdNameFlag := certificateNewVersionCmd.Flag("name", "certificate name in keyvault. Example: my-cert").Short('n').Required().String()
	certificateNewVersionSkipConfirmationFlag := certificateNewVersionCmd.Flag("skip-confirmation", "Create cert without prompting for confirmation").Bool()

	versionCmd := app.Command("version", "Print kvcrutch build and version information")

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
		err = glib.EditFile(embeddedConfig, *appConfigPathFlag, *configCmdEditCmdEditorFlag)
		if err != nil {
			sugarkane.Printw(os.Stderr,
				"ERROR: Unable to edit config",
				"configPath", *appConfigPathFlag,
				"editorPath", *configCmdEditCmdEditorFlag,
				"err", err,
			)
			return err
		}
		return nil
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

	// get a timeout
	timeout, err := time.ParseDuration(*appTimeout)
	if err != nil {
		err := errors.WithStack(err)
		sk.Errorw(
			"can't parse  --timeout",
			"err", err,
		)
		return err
	}

	// get the vaultURL
	vaultName := cfgVaultName
	if *appVaultNameFlag != "" {
		vaultName = *appVaultNameFlag
	}
	vaultFQDN := vaultName + ".vault.azure.net"
	port := "443"
	// Quick test to make sure we can connect
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: timeout},
		"tcp",
		net.JoinHostPort(vaultFQDN, port),
		nil,
	)
	if err != nil {
		err = errors.WithStack(err)
		sk.Errorw(
			"can't connect to vault",
			"vaultFQDN", vaultFQDN,
			"port", port,
			"timeout", timeout,
			"err", err,
		)
		return err
	}
	err = conn.Close()
	if err != nil {
		err = errors.WithStack(err)
		sk.Errorw(
			"can't close connection",
			"conn", conn,
			"err", err,
		)
		return err
	}
	vaultURL := "https://" + vaultFQDN

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
			return err
		}
		flagCertCreateParams := kvcrutch.FlagCertificateCreateParameters{
			Subject:          *certificateCreateCmdSubjectFlag,
			Sans:             *certificateCreateCmdSANsFlag,
			Tags:             flagTagsMap,
			ValidityInMonths: *certificateCreateCmdValidityInMonthsFlag,
			Enabled:          *certificateCreateCmdEnabledFlag,
			IssuerName:       *certificateCreateCmdIssuerNameFlag,
		}

		return kvcrutch.CertificateCreate(
			sk,
			kvClient,
			vaultURL,
			timeout,
			*certificateCreateCmdNameFlag,
			cfgCertCreateParams,
			flagCertCreateParams,
			*certificateCreateCmdNewVersionOkFlag,
			*certificateCreateCmdSkipConfirmationFlag,
		)

	case certificateListCmd.FullCommand():
		return kvcrutch.CertificateList(
			sk,
			kvClient,
			vaultURL,
			timeout,
		)
	case certificateNewVersionCmd.FullCommand():
		return kvcrutch.CertificateNewVersion(
			sk,
			kvClient,
			vaultURL,
			*certificateNewVersionCmdNameFlag,
			timeout,
			*certificateNewVersionSkipConfirmationFlag,
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
