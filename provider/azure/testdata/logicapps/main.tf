provider "azurerm" {
  features {}
}

resource "azurerm_resource_group" "rg" {
  name     = "rg"
  location = "East US"
}

# Counted: a request trigger, an HTTP action and a condition whose else
# branch calls a managed connector.
resource "azurerm_logic_app_workflow" "orders" {
  name                = "orders"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
}

resource "azurerm_logic_app_trigger_http_request" "orders" {
  name         = "request"
  logic_app_id = azurerm_logic_app_workflow.orders.id
  schema       = "{}"
}

resource "azurerm_logic_app_action_http" "notify" {
  name         = "notify"
  logic_app_id = azurerm_logic_app_workflow.orders.id
  method       = "POST"
  uri          = "https://example.com/notify"
}

resource "azurerm_logic_app_action_custom" "route" {
  name         = "route"
  logic_app_id = azurerm_logic_app_workflow.orders.id
  body = jsonencode({
    type   = "If"
    inputs = {}
    actions = { log = { type = "Compose", inputs = azurerm_logic_app_workflow.orders.id } }
    else = {
      actions = { mail = { type = "ApiConnection", inputs = {} } }
    }
  })
}

# A loop: its iterations are not in the definition.
resource "azurerm_logic_app_workflow" "batch" {
  name                = "batch"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
}

resource "azurerm_logic_app_action_custom" "each" {
  name         = "each"
  logic_app_id = azurerm_logic_app_workflow.batch.id
  body         = jsonencode({ type = "Foreach", foreach = "@triggerBody()", actions = {} })
}

# Defined in the designer: nothing in Terraform names it.
resource "azurerm_logic_app_workflow" "designer" {
  name                = "designer"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
}
