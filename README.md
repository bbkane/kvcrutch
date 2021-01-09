# kvcrutch

`kvcrutch` is a small tool for working with Azure Key Vaults and TLS
certificates. It's goal is to augment `az keyvault` in cases where `az
keyvault` isn't quite capable enough...

## Commands

### `kvcrutch certificate create`

`kvcrutch certificate create` exists because `az keyvault certificate create` requires you to type a new JSON creation policy each time you invoke it, which is error prone and annoying.

In contrast, `kvcrutch certificate create`:

- looks at a config file (use `kvcrutch config edit` to generate/edit a config) for certificate creation params
- overrides config created params with passed command line flags (note that some settings can only be toggled via config)
- checks if a certificate exists with the same ID
- prompts you before creating the certificate with relevant information

### `kvcrutch certificate list`

`kvcrutch certificate list` exists because `az keyvault certificate list` only returns the first 25 certificates in a Key Vault and then just stops...

This issue is tracked in https://github.com/Azure/azure-cli/issues/15382 and if that's resolved I might remove this command.

Here's a small script to download all certificates to JSON files in the current directory, which can be useful to grep if you're not sure which cert contains info you need.

```
$ kvcrutch certificate list | jq -r '.id' | while IFS='' read -r line || [ -n "${line}" ]; do
    az keyvault certificate show --id "$line" > "$(basename "$line").json"
done
```

## Install

### Homebrew

```
brew install bbkane/tap/kvcrutch
```

### Executables from GitHub

See the [releases](https://github.com/bbkane/kvcrutch/releases) to download an executable for Mac, Linux, or Windows.

### Build with [goreleaser](https://goreleaser.com/)

```
goreleaser --snapshot --skip-publish --rm-dist
```

### Build from source

Note that building this way doesn't embed the information `kvcrutch version` needs

```
go generate ./...
go build -tags=dist .
```
