# kvcrutch

`kvcrutch` is a small tool for working with Azure Key Vaults and TLS
certificates. It's goal is to complement `az keyvault ...`

Right now it's an easier and safer way to create TLS certificates.

`kvcrutch certificate create`:
- looks at a config file (use `kvcrutch config edit` to generate a config) for certificate creation params
- overrides config created params with passed command line flags (note that some settings can only be toggled via config)
- checks if a certificate exists with the same ID
- prompts you before creating the certificate with information to send

## Install

## Build
