package lib

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/2019-03-01/keyvault/keyvault"
	kvauth "github.com/Azure/azure-sdk-for-go/services/keyvault/auth"
	"github.com/Azure/go-autorest/autorest"
	"github.com/bbkane/kvcrutch/sugarkane"
	"github.com/pkg/errors"
)

type CfgCertificateCreateParameters struct {
	CertificateAttributes struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"certificate_attributes"`
	CertificatePolicy struct {
		KeyProperties struct {
			Exportable bool   `yaml:"exportable"`
			KeyType    string `yaml:"key_type"`
			KeySize    int32  `yaml:"key_size"`
			ReuseKey   bool   `yaml:"reuse_key"`
		} `yaml:"key_properties"`
		SecretProperties struct {
			ContentType string `yaml:"content_type"`
		} `yaml:"secret_properties"`
		X509CertificateProperties struct {
			Subject                 string   `yaml:"subject"`
			SubjectAlternativeNames []string `yaml:"subject_alternative_names"`
			ValidityInMonths        int32    `yaml:"validity_in_months"`
		} `yaml:"x509_certificate_properties"`
		LifetimeActions []struct {
			Trigger struct {
				LifetimePercentage *int32 `yaml:"lifetime_percentage"`
				DaysBeforeExpiry   *int32 `yaml:"days_before_expiry"`
			} `yaml:"trigger"`
			Action string `yaml:"action"`
		} `yaml:"lifetime_actions"`
		IssuerParameters struct {
			Name string `yaml:"name"`
		} `yaml:"issuer_parameters"`
	} `yaml:"certificate_policy"`
	Tags map[string]string `yaml:"tags"`
}

type FlagCertificateCreateParameters struct {
	Subject          string
	Sans             []string
	Tags             map[string]*string
	ValidityInMonths int32
	Enabled          bool
	IssuerName       string
}

func LogAutorestRequest(sk *sugarkane.SugarKane) autorest.PrepareDecorator {
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

func LogAutorestResponse(sk *sugarkane.SugarKane) autorest.RespondDecorator {
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

func CertificateCreate(
	sk *sugarkane.SugarKane,
	kvClient *keyvault.BaseClient,
	vaultURL string,
	timeout time.Duration,
	certName string,
	cfgCertCreateParams CfgCertificateCreateParameters,
	flagCertCreateParams FlagCertificateCreateParameters,
	newVersionOk bool,
	skipConfirmation bool,
) error {

	params := CreateKVCertCreateParamsFromCfg(cfgCertCreateParams)

	OverwriteKVCertCreateParamsWithCreateFlags(&params, flagCertCreateParams)

	// check if it exists - not that there's a small race condition if this succeeds and someone else creates
	// a cert with the name we want before we issue our create
	if !newVersionOk {
		// TODO: the timeout doesn't work here, though it work when I create a certificate
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		// A blank version means get the latest version
		// NOTE: how much $$$ does this call cost?
		_, err := kvClient.GetCertificate(ctx, vaultURL, certName, "")
		if err == nil {
			err = errors.Errorf("certificate already exists for certName: %#v\n", certName)
			sk.Errorw(
				"certificate already exists for name. Pass `--new-version-ok` to create a new version",
				"certName", certName,
				"err", err,
			)
			return err
		}
	}

	if !skipConfirmation {
		err := creationPrompt(vaultURL, &params)
		if err != nil {
			sk.Errorw(
				"Can't confirm creation",
				"vaultURL", vaultURL,
				"certName", certName,
				"err", err,
			)
			return err
		}

	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := kvClient.CreateCertificate(
		ctx,
		vaultURL,
		certName,
		params,
	)

	if err != nil {
		err = errors.WithStack(err)
		sk.Errorw(
			"certificate creation error",
			"err", err,
			"certName", certName,
		)
		return err
	}

	sk.Infow(
		"certificate created",
		"certName", certName,
		"createdID", *result.ID,
		"requestID", *result.RequestID,
		"status", *result.Status,
		"statusDetails", *result.StatusDetails,
	)
	return nil
}

func CreateKVCertCreateParamsFromCfg(cfgCCP CfgCertificateCreateParameters) keyvault.CertificateCreateParameters {

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

func OverwriteKVCertCreateParamsWithCreateFlags(
	ccp *keyvault.CertificateCreateParameters,
	flagCertCreateParams FlagCertificateCreateParameters) {

	if flagCertCreateParams.Subject != "" {
		ccp.CertificatePolicy.X509CertificateProperties.Subject = &flagCertCreateParams.Subject
	}
	if len(flagCertCreateParams.Sans) > 0 {
		ccp.CertificatePolicy.X509CertificateProperties.SubjectAlternativeNames.DNSNames = &flagCertCreateParams.Sans
	}
	if len(flagCertCreateParams.Tags) > 0 {
		ccp.Tags = flagCertCreateParams.Tags
	}
	if flagCertCreateParams.ValidityInMonths != 0 {
		ccp.CertificatePolicy.X509CertificateProperties.ValidityInMonths = &flagCertCreateParams.ValidityInMonths
	}
	// NOTE: if not passed, then this resolves as false
	// and we get the config version
	if flagCertCreateParams.Enabled != false {
		ccp.CertificateAttributes.Enabled = &flagCertCreateParams.Enabled
	}

	if flagCertCreateParams.IssuerName != "" {
		ccp.CertificatePolicy.IssuerParameters.Name = &flagCertCreateParams.IssuerName
	}
}

func ParseTags(flagTags []string) (map[string]*string, error) {
	flagTagsMap := make(map[string]*string)
	for _, kv := range flagTags {
		keyValue := strings.Split(kv, "=")
		if len(keyValue) != 2 {
			return flagTagsMap, errors.Errorf("tags should be formatted key=value : #%v", kv)
		}
		flagTagsMap[keyValue[0]] = &(keyValue[1])
	}
	return flagTagsMap, nil
}

func PrepareKV(sk *sugarkane.SugarKane) (*keyvault.BaseClient, error) {
	kvClient := keyvault.New()
	var err error
	kvClient.Authorizer, err = kvauth.NewAuthorizerFromCLI()
	if err != nil {
		err = errors.WithStack(err)
		sk.Errorw(
			"keyvault authorization error. Log in with `az login`",
			"err", err,
		)
		return nil, err
	}

	// https://github.com/Azure-Samples/azure-sdk-for-go-samples/blob/master/keyvault/examples/go-keyvault-msi-example.go
	kvClient.RequestInspector = LogAutorestRequest(sk)
	kvClient.ResponseInspector = LogAutorestResponse(sk)
	return &kvClient, nil
}

func CertificateList(sk *sugarkane.SugarKane, kvClient *keyvault.BaseClient, vaultURL string, timeout time.Duration) error {

	// TODO: this crosses boundaries as needed. If it does that lazily, will the context time out?
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	certs, err := kvClient.GetCertificatesComplete(ctx, vaultURL, nil)
	if err != nil {
		err = errors.WithStack(err)
		sk.Errorw(
			"Can't get certificates",
			"err", err,
		)
		return err
	}

	for certs.NotDone() {

		cert := certs.Value()

		certJSON, err := json.MarshalIndent(cert, "", "  ")
		if err != nil {
			err := errors.WithStack(err)
			sk.Errorw(
				"Can't marshall cert info",
				"cert", cert,
				"err", err,
			)
			return err
		}

		fmt.Println(string(certJSON))

		err = certs.NextWithContext(ctx)
		if err != nil {
			err := errors.WithStack(err)
			sk.Errorw(
				"Can't advance certs list",
				"certs", certs,
				"err", err,
			)
			return err
		}
	}

	return nil
}

func CertificateNewVersion(
	sk *sugarkane.SugarKane,
	kvClient *keyvault.BaseClient,
	vaultURL string,
	certName string,
	timeout time.Duration,
	skipConfirmation bool,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	certVersion := ""
	cert, err := kvClient.GetCertificate(ctx, vaultURL, certName, certVersion)
	if err != nil {
		err = errors.WithStack(err)
		sk.Errorw(
			"Can't get certificate",
			"vaultURL", vaultURL,
			"certName", certName,
			"err", err,
		)
		return err
	}

	certCreateParams := keyvault.CertificateCreateParameters{
		CertificatePolicy:     cert.Policy,
		CertificateAttributes: cert.Attributes,
		Tags:                  cert.Tags,
	}

	if !skipConfirmation {
		err := creationPrompt(vaultURL, &certCreateParams)
		if err != nil {
			sk.Errorw(
				"Can't confirm creation",
				"vaultURL", vaultURL,
				"certName", certName,
				"err", err,
			)
			return err
		}

	}

	ctx, cancel = context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := kvClient.CreateCertificate(
		ctx,
		vaultURL,
		certName,
		certCreateParams,
	)

	if err != nil {
		err = errors.WithStack(err)
		sk.Errorw(
			"certificate creation error",
			"err", err,
			"certName", certName,
		)
		return err
	}

	sk.Infow(
		"certificate created (new version)",
		"certName", certName,
		"createdID", *result.ID,
		"requestID", *result.RequestID,
		"status", *result.Status,
		"statusDetails", *result.StatusDetails,
	)

	return nil
}

func creationPrompt(vaultURL string, params *keyvault.CertificateCreateParameters) error {
	paramsJSON, err := json.MarshalIndent(
		params, "  ", "  ",
	)
	paramsJSONStr := string(paramsJSON)
	fmt.Printf("A certificate will be created in keyvault '%s' with the following parameters:\n", vaultURL)
	fmt.Print("  ")
	fmt.Println(paramsJSONStr)
	fmt.Print("Type 'yes' to continue: ")

	reader := bufio.NewReader(os.Stdin)
	confirmation, err := reader.ReadString('\n')
	confirmation = strings.TrimSpace(confirmation)
	if err != nil {
		err = errors.WithStack(err)
		return err
	}
	if confirmation != "yes" {
		err := errors.Errorf("confirmation not 'yes': %#v\n", confirmation)
		return err
	}
	return err
}
