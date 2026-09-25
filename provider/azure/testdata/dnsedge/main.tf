provider "azurerm" {
  features {}
}

resource "azurerm_linux_function_app" "app" {
  name     = "app"
  location = "japaneast"
}

resource "azurerm_frontdoor_firewall_policy" "waf" {
  name = "waf"

  custom_rule {
    name     = "block"
    priority = 1
  }

  managed_rule {
    type    = "DefaultRuleSet"
    version = "1.0"
  }

  managed_rule {
    type    = "Microsoft_BotManagerRuleSet"
    version = "1.0"
  }
}

resource "azurerm_frontdoor" "fd" {
  name = "fd"

  routing_rule {
    name = "a"
  }
  routing_rule {
    name = "b"
  }

  backend_pool {
    name = "pool"
    backend {
      address = azurerm_linux_function_app.app.default_hostname
    }
  }

  frontend_endpoint {
    name                                    = "fe"
    web_application_firewall_policy_link_id = azurerm_frontdoor_firewall_policy.waf.id
  }
}

resource "azurerm_dns_zone" "zone" {
  name = "example.com"
}

resource "azurerm_dns_mx_record" "mx" {
  name      = "@"
  zone_name = azurerm_dns_zone.zone.name
  record {
    preference = 10
    exchange   = "mx1.example.com"
  }
  record {
    preference = 20
    exchange   = "mx2.example.com"
  }
}

resource "azurerm_dns_a_record" "www" {
  name      = "www"
  zone_name = azurerm_dns_zone.zone.name
  records   = ["10.0.0.1", "10.0.0.2", "10.0.0.3"]
}

resource "azurerm_traffic_manager_profile" "tm" {
  name                 = "tm"
  traffic_view_enabled = true
}

resource "azurerm_traffic_manager_azure_endpoint" "ep" {
  name               = "ep"
  profile_id         = azurerm_traffic_manager_profile.tm.id
  target_resource_id = azurerm_linux_function_app.app.id
}
