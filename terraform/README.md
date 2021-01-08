# Terraform

## Run

Use `az login` to authenticate Terraform to Azure

Use the following environment variables:

```
# concert
export ARM_SUBSCRIPTION_ID='...'
```

```
terraform init
terraform apply
```

## Test Terraform Setup

### KeyVault

```
az keyvault show -n kvcrutch-kv-01-weus2-bbk -g kvcrutch-rg-01-weus2-bbk
```

## Azure Naming Conventions

Naming convention adapted from https://www.ironstoneit.com/blog/naming-conventions-for-azure
Resource short names: https://docs.microsoft.com/en-us/azure/cloud-adoption-framework/ready/azure-best-practices/naming-and-tagging
Location short names: http://www.mattruma.com/adventures-with-azure-regions/ (I'm gonna use `all` if it's not region scoped)

IronStone recommends: <Customer>-<Environment>-<Location>-<Service>-<InstanceNumber>-<Resource>

I want to put the fields I care about first because browser tabs don't show the full thing yo :)

I'mma go with <project>-<resource-type-short-name>-<instance-number>-<environment>-<location-short-name>-<owner>

Examples: kvcrutch-rg-01-dev-weus2-bbkane

