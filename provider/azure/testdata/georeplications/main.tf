resource "azurerm_container_registry" "acr" {
  name                = "acr"
  resource_group_name = "rg"
  location            = "japaneast"
  sku                 = "Premium"

  georeplications {
    location = "japanwest"
  }

  dynamic "georeplications" {
    for_each = ["eastus"]
    content {
      location = georeplications.value
    }
  }
}

resource "azurerm_container_registry" "plain" {
  name                = "plain"
  resource_group_name = "rg"
  location            = "japaneast"
  sku                 = "Basic"
}
