provider "azurerm" {
  features {}
}

resource "azurerm_mssql_server" "sql" {
  name                = "sql"
  location            = "japaneast"
  resource_group_name = "rg"
  version             = "12.0"
}

resource "azurerm_mssql_elasticpool" "pool" {
  name                = "pool"
  location            = "japaneast"
  resource_group_name = "rg"
  server_name         = azurerm_mssql_server.sql.name
  max_size_gb         = 64

  sku {
    name     = "GP_Gen5"
    tier     = "GeneralPurpose"
    family   = "Gen5"
    capacity = 4
  }

  per_database_settings {
    min_capacity = 0
    max_capacity = 4
  }
}

# In the pool: the ID exists only after apply, and the pool bills compute.
resource "azurerm_mssql_database" "pooled" {
  name            = "pooled"
  server_id       = azurerm_mssql_server.sql.id
  elastic_pool_id = azurerm_mssql_elasticpool.pool.id
}

resource "azurerm_mssql_database" "single" {
  name      = "single"
  server_id = azurerm_mssql_server.sql.id
  sku_name  = "S0"
}
