provider "azurerm" {
  features {}
}

resource "azurerm_storage_account" "data" {
  name                     = "data"
  location                 = "Japan East"
  account_tier             = "Standard"
  account_replication_type = "LRS"
}

resource "azurerm_servicebus_namespace" "bus" {
  name     = "bus"
  location = "japaneast"
  sku      = "Standard"
}

resource "azurerm_user_assigned_identity" "worker" {
  name     = "worker"
  location = "japaneast"
}

resource "azurerm_service_plan" "plan" {
  name     = "plan"
  location = "japaneast"
  os_type  = "Linux"
  sku_name = "Y1"
}

# A system-assigned identity reads the blobs; a user-assigned identity sends
# to the bus. The storage account named for the runtime is not a data call.
resource "azurerm_linux_function_app" "api" {
  name                 = "api"
  location             = "japaneast"
  service_plan_id      = azurerm_service_plan.plan.id
  storage_account_name = azurerm_storage_account.data.name
  identity {
    type         = "SystemAssigned, UserAssigned"
    identity_ids = [azurerm_user_assigned_identity.worker.id]
  }
}

resource "azurerm_role_assignment" "read_blobs" {
  scope                = azurerm_storage_account.data.id
  role_definition_name = "Storage Blob Data Reader"
  principal_id         = azurerm_linux_function_app.api.identity[0].principal_id
}

resource "azurerm_role_assignment" "send" {
  scope                = azurerm_servicebus_namespace.bus.id
  role_definition_name = "Azure Service Bus Data Sender"
  principal_id         = azurerm_user_assigned_identity.worker.principal_id
}
