terraform {
  required_providers {
    proxmox = {
      source = "local.dev/mollstam/proxmox"
    }
  }
}

provider "proxmox" {
  api_url = "https://172.26.56.125:8006/api2/json"

  debug        = true
  tls_insecure = true
}

resource "proxmox_vm" "example" {
  node = "pve"
  name = "alice"

  virtio0 = {
    media   = "disk"
    size    = 30
    storage = "local-lvm"
  }
}
