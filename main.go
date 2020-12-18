//go:generate go run ./generate_static_data.go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/bbkane/kvcrutch/static"
	"github.com/bbkane/kvcrutch/sugarkane"

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/keyvault/keyvault"
	kvauth "github.com/Azure/azure-sdk-for-go/services/keyvault/auth"
	"github.com/Azure/go-autorest/autorest"
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

// config stuff

type keyProperties struct {
	Exportable bool   `yaml:"exportable"`
	KeyType    string `yaml:"key_type"`
	KeySize    int32  `yaml:"key_size"`
	ReuseKey   bool   `yaml:"reuse_key"`
}

type secretProperties struct {
	ContentType string `yaml:"content_type"`
}

type x509CertificateProperties struct {
	Subject                 string   `yaml:"subject"`
	SubjectAlternativeNames []string `yaml:"subject_alternative_names"`
	ValidityInMonths        int32    `yaml:"validity_in_months"`
}

type trigger struct {
	LifetimePercentage *int32 `yaml:"lifetime_percentage"`
	DaysBeforeExpiry   *int32 `yaml:"days_before_expiry"`
}

type lifetimeAction struct {
	Trigger trigger `yaml:"trigger"`
	Action  string  `yaml:"action"`
}

type issuerParameters struct {
	Name string `yaml:"name"`
}

type certificatePolicy struct {
	KeyProperties             keyProperties             `yaml:"key_properties"`
	SecretProperties          secretProperties          `yaml:"secret_properties"`
	X509CertificateProperties x509CertificateProperties `yaml:"x509_certificate_properties"`
	LifetimeActions           []lifetimeAction          `yaml:"lifetime_actions"`
	IssuerParameters          issuerParameters          `yaml:"issuer_parameters"`
}

type certificateAttributes struct {
	Enabled bool `yaml:"enabled"`
}

type certificateCreateParameters struct {
	CertificateAttributes certificateAttributes `yaml:"certificate_attributes"`
	CertificatePolicy     certificatePolicy     `yaml:"certificate_policy"`
	Tags                  map[string]string     `yaml:"tags"`
}

type config struct {
	Version                     string
	LumberjackLogger            *lumberjack.Logger          `yaml:"lumberjacklogger"`
	VaultName                   string                      `yaml:"vault_name"`
	CertificateCreateParameters certificateCreateParameters `yaml:"certificate_create_parameters"`
}

func parseConfig(configBytes []byte) (*lumberjack.Logger, string, certificateCreateParameters, error) {

	cfg := config{}
	err := yaml.UnmarshalStrict(configBytes, &cfg)
	if err != nil {
		// not ok to get invalid YAML
		return nil, "", certificateCreateParameters{}, errors.WithStack(err)
	}

	var lumberjackLogger *lumberjack.Logger = nil

	// we can get a valid config with a nil logger
	if cfg.LumberjackLogger != nil {
		// Note that if the directories to here don't exist, lumberjack will
		// make them
		f, err := homedir.Expand(cfg.LumberjackLogger.Filename)
		if err != nil {
			return nil, "", certificateCreateParameters{}, errors.WithStack(err)
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

	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		err = ioutil.WriteFile(configPath, defaultConfig, 0644)
		if err != nil {
			sugarkane.Printw(os.Stderr,
				"can't write config",
				"err", err,
			)
			return err
		}
		sugarkane.Printw(os.Stdout,
			"wrote default config",
			"configPath", configPath,
		)
	} else if err != nil {
		sugarkane.Printw(os.Stderr,
			"can't write config",
			"err", err,
		)
		return err
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

func logAutorestRequest(sk *sugarkane.SugarKane) autorest.PrepareDecorator {
	return func(p autorest.Preparer) autorest.Preparer {
		return autorest.PreparerFunc(func(r *http.Request) (*http.Request, error) {
			r, err := p.Prepare(r)
			if err != nil {
				err := errors.WithStack(err)
				sk.Errorw(
					"autorest HTTP request error",
					"err", err,
				)
			}
			dump, _ := httputil.DumpRequestOut(r, true)
			sk.Debugw(
				"autorest HTTP request",
				"req", string(dump),
			)
			return r, err
		})
	}
}

func logAutorestResponse(sk *sugarkane.SugarKane) autorest.RespondDecorator {
	return func(p autorest.Responder) autorest.Responder {
		return autorest.ResponderFunc(func(r *http.Response) error {
			err := p.Respond(r)
			if err != nil {
				err := errors.WithStack(err)
				sk.Errorw(
					"autorest HTTP response error",
					"err", err,
				)
			}
			dump, _ := httputil.DumpResponse(r, true)
			sk.Debugw(
				"autorest HTTP response",
				"req", string(dump),
			)
			return err
		})
	}
}

func createKVCertCreateParamsFromCfg(cfgCCP certificateCreateParameters) keyvault.CertificateCreateParameters {

	var la []keyvault.LifetimeAction
	{
		for _, e := range cfgCCP.CertificatePolicy.LifetimeActions {
			la = append(la, keyvault.LifetimeAction{
				Trigger: &keyvault.Trigger{
					LifetimePercentage: e.Trigger.LifetimePercentage,
					DaysBeforeExpiry:   e.Trigger.DaysBeforeExpiry,
				},
				Action: &keyvault.Action{
					ActionType: keyvault.ActionType(e.Action),
				},
			})
		}
	}

	tags := make(map[string]*string)
	{
		for k, v := range cfgCCP.Tags {
			tags[k] = &v
		}
	}

	ccp := keyvault.CertificateCreateParameters{
		CertificateAttributes: &keyvault.CertificateAttributes{
			Enabled: &cfgCCP.CertificateAttributes.Enabled,
		},
		CertificatePolicy: &keyvault.CertificatePolicy{
			ID: nil,
			KeyProperties: &keyvault.KeyProperties{
				Exportable: &cfgCCP.CertificatePolicy.KeyProperties.Exportable,
				KeyType:    &cfgCCP.CertificatePolicy.KeyProperties.KeyType,
				KeySize:    &cfgCCP.CertificatePolicy.KeyProperties.KeySize,
				ReuseKey:   &cfgCCP.CertificatePolicy.KeyProperties.ReuseKey,
			},
			SecretProperties: &keyvault.SecretProperties{
				ContentType: &cfgCCP.CertificatePolicy.SecretProperties.ContentType,
			},
			X509CertificateProperties: &keyvault.X509CertificateProperties{
				Subject: &cfgCCP.CertificatePolicy.X509CertificateProperties.Subject,
				Ekus:    nil,
				SubjectAlternativeNames: &keyvault.SubjectAlternativeNames{
					DNSNames: &cfgCCP.CertificatePolicy.X509CertificateProperties.SubjectAlternativeNames,
				},
				KeyUsage:         nil,
				ValidityInMonths: &cfgCCP.CertificatePolicy.X509CertificateProperties.ValidityInMonths,
			},
			LifetimeActions: &la,
			IssuerParameters: &keyvault.IssuerParameters{
				Name:            &cfgCCP.CertificatePolicy.IssuerParameters.Name,
				CertificateType: nil,
			},
			Attributes: nil,
		},
		Tags: tags,
	}

	return ccp
}

func overwriteKVCertCreateParamsWithCreateFlags(
	ccp *keyvault.CertificateCreateParameters,
	flagSubject string,
	flagSans []string,
	flagTagsMap map[string]*string,
	flagValidityInMonths int32,
	flagEnabled bool) {

	if flagSubject != "" {
		ccp.CertificatePolicy.X509CertificateProperties.Subject = &flagSubject
	}
	if len(flagSans) > 0 {
		ccp.CertificatePolicy.X509CertificateProperties.SubjectAlternativeNames.DNSNames = &flagSans
	}
	if len(flagTagsMap) > 0 {
		ccp.Tags = flagTagsMap
	}
	if flagValidityInMonths != 0 {
		ccp.CertificatePolicy.X509CertificateProperties.ValidityInMonths = &flagValidityInMonths
	}
	// NOTE: if not passed, then this resolves as false
	// and we get the config version
	if flagEnabled != false {
		ccp.CertificateAttributes.Enabled = &flagEnabled
	}

}

func certificateCreate(
	sk *sugarkane.SugarKane,
	cfgVaultName string,
	cfgCertificateCreateParams certificateCreateParameters,
	flagVaultName string,
	flagID string,
	flagSubject string,
	flagSans []string,
	flagTags []string,
	flagValidityInMonths int32,
	flagEnabled bool,
	flagNewVersionOk bool,
) error {

	params := createKVCertCreateParamsFromCfg(cfgCertificateCreateParams)

	flagTagsMap := make(map[string]*string)
	for _, kv := range flagTags {
		keyValue := strings.Split(kv, "=")
		if len(keyValue) != 2 {
			return errors.Errorf("tags should be formatted key=value : #%v", kv)
		}
		flagTagsMap[keyValue[0]] = &(keyValue[1])
	}
	overwriteKVCertCreateParamsWithCreateFlags(
		&params, flagSubject, flagSans, flagTagsMap, flagValidityInMonths, flagEnabled)

	kvClient := keyvault.New()
	var err error
	kvClient.Authorizer, err = kvauth.NewAuthorizerFromCLI()
	if err != nil {
		err = errors.WithStack(err)
		sk.Errorw(
			"keyvault authorization error",
			"err", err,
		)
		return err
	}

	// https://github.com/Azure-Samples/azure-sdk-for-go-samples/blob/master/keyvault/examples/go-keyvault-msi-example.go
	kvClient.RequestInspector = logAutorestRequest(sk)
	kvClient.ResponseInspector = logAutorestResponse(sk)

	vaultName := cfgVaultName
	if flagVaultName != "" {
		vaultName = flagVaultName
	}
	baseURL := "https://" + vaultName + ".vault.azure.net"

	// check if it exists - not that there's a small race condition if this succeeds and someone else creates
	// a cert with the id we want before we issue our create
	if !flagNewVersionOk {
		// A blank version means get the latest version
		cert, err := kvClient.GetCertificate(context.Background(), baseURL, flagID, "")
		pretendToUse(cert)
		if err == nil {
			err = errors.Errorf("certificate already exists for id: %#v\n", flagID)
			sk.Errorw(
				"certificate already exists for id. Pass `--new-version-ok` to create a new version",
				"id", flagID,
				"err", err,
			)
			return err
		}
	}

	// ask for confirmation
	{
		paramsJSON, err := json.MarshalIndent(
			params, "  ", "  ",
		)
		paramsJSONStr := string(paramsJSON)
		fmt.Printf("A certificate will be created in keyvault '%s' with the following parameters:\n", vaultName)
		fmt.Print("  ")
		fmt.Println(paramsJSONStr)
		fmt.Print("Type 'yes' to continue: ")

		reader := bufio.NewReader(os.Stdin)
		confirmation, err := reader.ReadString('\n')
		confirmation = strings.TrimSpace(confirmation)
		if err != nil {
			err = errors.WithStack(err)
			sk.Errorw(
				"Cannot read confirmation input",
				"err", err,
			)
			return err
		}
		if confirmation != "yes" {
			err := errors.Errorf("confirmation not 'yes': %#v\n", confirmation)
			sk.Errorw(
				"confirmation went bad",
				"confirmation", confirmation,
				"err", err,
			)
			return err
		}
	}

	// kvClient.
	result, err := kvClient.CreateCertificate(
		context.Background(),
		baseURL,
		flagID,
		params,
	)

	if err != nil {
		err = errors.WithStack(err)
		sk.Errorw(
			"certificate creation error",
			"err", err,
			"id", flagID,
		)
		return err
	}

	sk.Infow(
		"certificate created",
		"certId", flagID,
		"createdID", *result.ID,
		"requestID", *result.RequestID,
		"status", *result.Status,
		"statusDetails", *result.StatusDetails,
	)
	return nil
}

func run() error {

	// parse the CLI args
	app := kingpin.New("kvcrutch", "Lean on me when `az keyvault` isn't quite as useful as needed").UsageTemplate(kingpin.DefaultUsageTemplate)
	app.HelpFlag.Short('h')
	defaultConfigPath := "~/.config/kvcrutch.yaml"
	appConfigPathFlag := app.Flag("config-path", "config filepath").Short('c').Default(defaultConfigPath).String()
	appVaultNameFlag := app.Flag("vault-name", "Key Vault Name").Short('v').String()

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
	certificateCreateCmdEnabledFlag := certificateCreateCmd.Flag("enabled", "enable certificate on creation").Short('d').Bool()
	certificateCreateCmdNewVersionOkFlag := certificateCreateCmd.Flag("new-version-ok", "Confirm it's ok to create a new version of a certificate").Short('n').Bool()

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

	// dispatch commands that use dependencies
	switch cmd {
	case certificateCreateCmd.FullCommand():
		return certificateCreate(
			sk,
			cfgVaultName,
			cfgCertCreateParams,
			*appVaultNameFlag,
			*certificateCreateCmdIDFlag,
			*certificateCreateCmdSubjectFlag,
			*certificateCreateCmdSANsFlag,
			*certificateCreateCmdTagsFlag,
			*certificateCreateCmdValidityInMonthsFlag,
			*certificateCreateCmdEnabledFlag,
			*certificateCreateCmdNewVersionOkFlag,
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
