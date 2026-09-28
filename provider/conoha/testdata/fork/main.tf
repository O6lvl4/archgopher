resource "conohavps_instance" "web" {
  flavor_id         = "f2a77529-1815-43a2-bc14-1f3f6b09079c"
  instance_name_tag = "web"
  key_name          = conohavps_keypair.deploy.name
  block_device {
    uuid = conohavps_volume.boot.id
  }
}

resource "conohavps_keypair" "deploy" {
  name       = "deploy"
  public_key = "ssh-ed25519 AAAA"
}

resource "conohavps_volume" "boot" {
  name        = "boot"
  size        = 100
  volume_type = "c3j1-ds02-boot"
  image_ref   = "11111111-1111-1111-1111-111111111111"
}

resource "conohavps_volume" "data" {
  name        = "data"
  size        = 200
  volume_type = "c3j1-ds02-add"
}

resource "conohavps_volume_attachment" "data" {
  instance_id = conohavps_instance.web.id
  volume_id   = conohavps_volume.data.id
}

resource "conohavps_additional_ip" "extra" {
  ip_count = 2
}

resource "conohavps_port_attachment" "extra" {
  server_id = conohavps_instance.web.id
  port_id   = conohavps_additional_ip.extra.id
}

resource "conohavps_instance_autobackup" "backup" {
  instance_id = conohavps_instance.web.id
  retention   = 20
}

resource "conohavps_lb_loadbalancer" "lb" {
  name = "lb"
}

resource "conohavps_lb_listener" "http" {
  loadbalancer_id = conohavps_lb_loadbalancer.lb.id
  name            = "http"
  protocol        = "TCP"
  protocol_port   = 80
}

resource "conohavps_lb_pool" "web" {
  listener_id  = conohavps_lb_listener.http.id
  name         = "web"
  protocol     = "TCP"
  lb_algorithm = "ROUND_ROBIN"
}

resource "conohavps_lb_member" "web" {
  pool_id       = conohavps_lb_pool.web.id
  name          = "web"
  address       = conohavps_instance.web.addresses["ext-100"][0].addr
  protocol_port = 80
}

resource "conohavps_lb_health_monitor" "web" {
  pool_id     = conohavps_lb_pool.web.id
  name        = "tcp"
  type        = "TCP"
  delay       = 10
  timeout     = 5
  max_retries = 3
}

resource "conohavps_network" "local" {}

resource "conohavps_subnet" "local" {
  network_id = conohavps_network.local.id
  cidr       = "10.0.0.0/24"
}

resource "conohavps_port" "local" {
  network_id = conohavps_network.local.id
}

resource "conohavps_port_attachment" "local" {
  server_id = conohavps_instance.web.id
  port_id   = conohavps_port.local.id
}

resource "conohavps_image_quota" "images" {
  image_size_gb = 550
}

resource "conohavps_objectstorage_quota" "objects" {
  quota_gb = 200
}

resource "conohavps_objectstorage_container" "assets" {
  name = "assets"
}

resource "conohavps_dns_domain" "example" {
  name  = "example.com."
  email = "admin@example.com"
  ttl   = 3600
}

resource "conohavps_dns_record" "www" {
  domain_id = conohavps_dns_domain.example.id
  name      = "www.example.com."
  type      = "A"
  data      = conohavps_lb_loadbalancer.lb.vip_address
}
