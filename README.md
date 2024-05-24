
# Terraform provider for Proxmox Virtual Environment

:warning: **This is provider is very much under development. Feature set pretty much only includes what I currently need. Provided as-is but let's see where we can take it, together. :sparkles:**

This is a fork of [Telmate/terraform-provider-proxmox](https://github.com/Telmate/terraform-provider-proxmox) where I was just going to look into some state drift issues but ended up deleting all resource and rewriting them from, with some significant changes:

- :mountain: Migrated to **Terraform Plugin Framework** instead of the old **SDKv2**
- :fried_egg: Simplified naming, e.g. `proxmox_vm` instead of `proxmox_vm_qemu`
- :dragon: Vehemently opposes state drift and reconciles managed state that was changed out-of-band, eg in Proxmox Web UI
- :boom: Currently no guarantees that it won't destroy your infra, see warning at the very top

:point_up: It started out nice and clean but as things grow so does complexity. Could use a bit of a restructure but I need to get on with my todo list now. At least we have a bunch of acceptance tests. Feel like this isn't something one puts in the read-me...

## Testing

To run acceptance tests I recommend you set up e.g. a VirualBox VM with PVE. I recommend running VM with NAT network and port forwarding 8806 to VM:8006, that's whats currently configured in the test code, for other setups you'll have to make local changes. There should also be a VNet with DHCP doling out addresses somewhere in `10.0.0.0/24`, and a template with VMID 300 that has the `qemu-guest-agent` installed.

Sorry for the artisinal setup, to be cleaned up.

## Contribute

Contributions welcome! :sparkling_heart:

Follow the [Code of Conduct](CODE-OF-CONDUCT.md).

## Useful links

* [Proxmox](https://www.proxmox.com/en/)
* [Proxmox documentation](https://pve.proxmox.com/pve-docs/)
* [Terraform](https://www.terraform.io/)
* [Terraform documentation](https://www.terraform.io/docs/index.html)
* [Recommended ISO builder](https://github.com/Telmate/terraform-ubuntu-proxmox-iso)
