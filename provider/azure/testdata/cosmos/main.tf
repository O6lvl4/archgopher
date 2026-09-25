provider "azurerm" {
  features {}
}

resource "azurerm_cosmosdb_account" "store" {
  name       = "store"
  location   = "japaneast"
  offer_type = "Standard"
}

resource "azurerm_cosmosdb_sql_database" "shop" {
  name         = "shop"
  account_name = azurerm_cosmosdb_account.store.name
  throughput   = 400
}

resource "azurerm_cosmosdb_sql_container" "orders" {
  name                = "orders"
  account_name        = azurerm_cosmosdb_account.store.name
  database_name       = azurerm_cosmosdb_sql_database.shop.name
  partition_key_paths = ["/id"]
  autoscale_settings {
    max_throughput = 4000
  }
}

resource "azurerm_cosmosdb_cassandra_keyspace" "log" {
  name         = "log"
  account_name = azurerm_cosmosdb_account.store.name
}

resource "azurerm_cosmosdb_cassandra_table" "events" {
  name                  = "events"
  cassandra_keyspace_id = azurerm_cosmosdb_cassandra_keyspace.log.id
  throughput            = 400
}

resource "azurerm_cosmosdb_account" "archive" {
  name       = "archive"
  location   = "japaneast"
  offer_type = "Standard"
}

# A database read as a data source is not a node, but a call to it still
# reaches its account.
data "azurerm_cosmosdb_sql_database" "old" {
  name                = "old"
  resource_group_name = "rg"
  account_name        = azurerm_cosmosdb_account.archive.name
}

resource "azurerm_service_plan" "plan" {
  name     = "plan"
  location = "japaneast"
  os_type  = "Linux"
  sku_name = "Y1"
}

# The app names the container, its database and the table, never the account.
resource "azurerm_linux_function_app" "api" {
  name            = "api"
  location        = "japaneast"
  service_plan_id = azurerm_service_plan.plan.id
  app_settings = {
    COSMOS_DATABASE  = azurerm_cosmosdb_sql_database.shop.name
    COSMOS_CONTAINER = azurerm_cosmosdb_sql_container.orders.name
    EVENTS_TABLE     = azurerm_cosmosdb_cassandra_table.events.name
    ARCHIVE_DATABASE = data.azurerm_cosmosdb_sql_database.old.name
  }
}
