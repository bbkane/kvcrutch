terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 2.37.0"
    }
  }
  required_version = ">= 0.13"
}

# https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs
provider "azurerm" {
  # export ARM_SUBSCRIPTION_ID=...
  features {}
}

variable "environment" {
  type = string
  description = "dev, test, prod..."
}
variable "location" {
  type = string
  description = "Azure location"
}

variable "location_short" {
  type= string
  description = "shorter version of `location`"
}


variable "owner" {
  type = string
  description = "Owner of the project"
}

variable "project" {
  type = string
  description = "name of the project"
}


# https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/data-sources/client_config
# Basically the user using Terraform
# TODO: replace this with terraform.tfvars
data "azurerm_client_config" "client_config" {}

resource "azurerm_resource_group" "resource_group" {
  name     = "${var.project}-rg-01-${var.environment}-${var.location_short}-${var.owner}"
  location = var.location
}

# https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/key_vault
resource "azurerm_key_vault" "key_vault" {
  name                = "${var.project}-kv-01-${var.environment}-${var.location_short}-${var.owner}"
  location            = azurerm_resource_group.resource_group.location
  resource_group_name = azurerm_resource_group.resource_group.name
  tenant_id           = data.azurerm_client_config.client_config.tenant_id

  # https://docs.microsoft.com/en-us/azure/key-vault/general/key-vault-recovery?tabs=azure-portal
  soft_delete_enabled = false # I want to be able to blow this away :)
  # soft_delete_retention_days  = 7
  # purge_protection_enabled    = false

  sku_name = "standard"

  # Give my account all access
  access_policy {
    tenant_id = data.azurerm_client_config.client_config.tenant_id
    object_id = data.azurerm_client_config.client_config.object_id

    certificate_permissions = [
      "backup", "create", "delete", "deleteissuers", "get", "getissuers", "import", "list", "listissuers", "managecontacts", "manageissuers", "purge", "recover", "restore", "setissuers", "update"
    ]

    key_permissions = [
      "backup", "create", "decrypt", "delete", "encrypt", "get", "import", "list", "purge", "recover", "restore", "sign", "unwrapKey", "update", "verify", "wrapKey"
    ]

    secret_permissions = [
      "backup", "delete", "get", "list", "purge", "recover", "restore", "set"
    ]

    storage_permissions = [
      "backup", "delete", "deletesas", "get", "getsas", "list", "listsas", "purge", "recover", "regeneratekey", "restore", "set", "setsas", "update"
    ]
  }
}

# https://stackoverflow.com/questions/53991906/how-can-i-use-terraform-to-create-a-service-principal-and-use-that-principal-in
# https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/role_assignment
resource "azurerm_role_assignment" "role_assignment" {
  scope                = azurerm_key_vault.key_vault.id # the resource id
  role_definition_name = "Contributor"                  # such as "Contributor"
  principal_id         = data.azurerm_client_config.client_config.object_id
}

