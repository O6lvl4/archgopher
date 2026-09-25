provider "google" {
  region = "asia-northeast1"
}

resource "google_compute_instance_template" "web" {
  machine_type = "n2-standard-4"
  scheduling {
    provisioning_model = "SPOT"
  }
  disk {
    source_image = "debian-cloud/debian-12"
    disk_type    = "pd-balanced"
    disk_size_gb = 20
  }
  network_interface {
    network = "default"
    access_config {}
  }
}

resource "google_compute_instance_group_manager" "web" {
  name        = "web"
  zone        = "asia-northeast1-a"
  target_size = 3
  version {
    instance_template = google_compute_instance_template.web.id
  }
}

resource "google_compute_per_instance_config" "pinned" {
  instance_group_manager = google_compute_instance_group_manager.web.name
  name                   = "pinned"
}

resource "google_compute_disk" "data" {
  name = "data"
  type = "pd-ssd"
  size = 200
}

resource "google_compute_instance" "db" {
  name         = "db"
  machine_type = "n1-standard-2"
  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-12"
    }
  }
  attached_disk {
    source = google_compute_disk.data.id
  }
  scratch_disk {
    interface = "NVME"
  }
  scratch_disk {
    interface = "NVME"
  }
  network_interface {
    network = "default"
  }
}

resource "google_container_cluster" "main" {
  name                     = "main"
  location                 = "asia-northeast1"
  remove_default_node_pool = true
  initial_node_count       = 1
  node_locations           = ["asia-northeast1-a", "asia-northeast1-b"]
}

resource "google_container_node_pool" "general" {
  name       = "general"
  cluster    = google_container_cluster.main.id
  node_count = 2
  node_config {
    machine_type = "e2-standard-4"
  }
}
