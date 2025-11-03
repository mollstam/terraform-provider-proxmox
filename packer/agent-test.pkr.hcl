
source "proxmox-iso" "agent-test" {
  proxmox_url              = "https://172.26.56.125:8006/api2/json"
  username                 = "root@pam"
  password                 = "123123"
  insecure_skip_tls_verify = true

  node    = "pve"
  vm_id   = "300"
  vm_name = "agent-test-template"

  iso_file         = "local:iso/ubuntu-22.04.4-live-server-amd64.iso"
  iso_storage_pool = "local"
  unmount_iso      = true

  qemu_agent      = true
  scsi_controller = "virtio-scsi-pci"
  cores           = "1"
  memory          = "2048"

  disks {
    disk_size    = "20G"
    storage_pool = "local-lvm"
    type         = "virtio"
  }

  network_adapters {
    model  = "virtio"
    bridge = "vmbr0"
  }
  
    additional_iso_files {
    iso_storage_pool = "local"
    unmount          = true
    cd_files         = []
    cd_content = {
      "user-data" = <<EOT
#cloud-config
autoinstall:
  version: 1
  locale: en_US
  identity:
    hostname: ubuntu-server
    password: "$6$exDY1mhS4KUYCE/2$zmn9ToZwTKLhCw.b4/b.ZRTIZM30JZ4QrOQ2aOXJ8yk96xpcCof0kxKwuX1kqLG/ygbJ1f8wxED22bTL4F46P0"
    username: ubuntu
  ssh:
    install-server: yes
    allow-pw: yes
  packages:
  - qemu-guest-agent
  user-data:
    disable_root: false
EOT
      "meta-data" = ""
    }
    cd_label = "cidata"
  }

  boot_command = [
    "e<down><down><down><end><wait>",
    " autoinstall<wait> ds=nocloud;",
    "<F10>",
  ]

  ssh_username = "ubuntu"
  ssh_password = "ubuntu"
  ssh_timeout  = "15m"
}

build {
  name    = "agent-test-template"
  sources = ["sources.proxmox-iso.agent-test"]
}