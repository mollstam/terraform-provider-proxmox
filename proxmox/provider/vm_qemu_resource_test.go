package provider

import (
	"fmt"
	"strconv"
	"testing"

	pveapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/mollstam/terraform-provider-proxmox/proxmox/provider/testutil"
	. "github.com/onsi/gomega"
)

func TestAccVmQemuResource_CreateAndUpdate(t *testing.T) {
	var vm vmQemuResourceModel

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
resource "proxmox_vm_qemu" "test" {
	node = "pve"
	name = "wall-e"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVmQemuExistsInPve("proxmox_vm_qemu.test", &vm),
					testAccCheckVmQemuValuesInPve(&vm, "pve", 100, "wall-e"),
					resource.TestCheckResourceAttr("proxmox_vm_qemu.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm_qemu.test", "vmid", "100"),
					resource.TestCheckResourceAttr("proxmox_vm_qemu.test", "name", "wall-e"),
				),
			},
			// Update and Read testing
			{
				Config: providerConfig + `
resource "proxmox_vm_qemu" "test" {
	node = "pve"
	name = "m-o"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVmQemuExistsInPve("proxmox_vm_qemu.test", &vm),
					testAccCheckVmQemuValuesInPve(&vm, "pve", 100, "m-o"),
					resource.TestCheckResourceAttr("proxmox_vm_qemu.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm_qemu.test", "vmid", "100"),
					resource.TestCheckResourceAttr("proxmox_vm_qemu.test", "name", "m-o"),
				),
			},
		},
	})
}

func TestAccVmQemuResource_RefreshDeletedVm(t *testing.T) {
	var vm vmQemuResourceModel

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
resource "proxmox_vm_qemu" "test" {
	node = "pve"
	name = "wall-e"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckVmQemuExistsInPve("proxmox_vm_qemu.test", &vm),
				),
			},
			{
				RefreshState:       true,
				PreConfig:          testAccDeleteVmInPve(&vm),
				ExpectNonEmptyPlan: true,
			},
			/*{
							Config: providerConfig + `
			resource "proxmox_vm_qemu" "test" {
				node = "pve"
				name = "wall-e"
			}
			`,
							Check: resource.ComposeTestCheckFunc(
								testAccCheckVmQemuExistsInPve("proxmox_vm_qemu.test", &vm),
							),
						},*/
		},
	})
}

func testAccCheckVmQemuExistsInPve(n string, r *vmQemuResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		vmid, err := strconv.ParseInt(rs.Primary.Attributes["vmid"], 10, 64)
		if err != nil {
			return err
		}

		ref := pveapi.NewVmRef(int(vmid))
		config, err := pveapi.NewConfigQemuFromApi(ref, testutil.TestClient)
		if err != nil {
			return err
		}

		ResourceModelFromConfig(config, r)

		return nil
	}
}

func testAccCheckVmQemuValuesInPve(r *vmQemuResourceModel, node string, vmid int64, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		err := InterceptGomegaFailure(func() {
			Expect(r.Node.IsUnknown()).To(BeFalseBecause("Node should be a known value"))
			Expect(r.Node.IsNull()).To(BeFalseBecause("Node should not be null"))
			Expect(r.Node.ValueString()).To(Equal(node))

			Expect(r.VMID.IsUnknown()).To(BeFalseBecause("VMID should be a known value"))
			Expect(r.VMID.IsNull()).To(BeFalseBecause("VMID should not be null"))
			Expect(r.VMID.ValueInt64()).To(Equal(vmid))

			Expect(r.Name.IsUnknown()).To(BeFalseBecause("Name should be a known value"))
			Expect(r.Name.IsNull()).To(BeFalseBecause("Name should not be null"))
			Expect(r.Name.ValueString()).To(Equal(name))
		})
		if err != nil {
			return err
		}

		return nil
	}
}

func testAccDeleteVmInPve(r *vmQemuResourceModel) func() {
	return func() {
		ref := pveapi.NewVmRef(int(r.VMID.ValueInt64()))
		ref.SetNode(r.Node.ValueString())

		_, err := testutil.TestClient.DeleteVm(ref)
		if err != nil {
			panic(fmt.Sprint("Failed to delete VM during test step: " + err.Error()))
		}
	}
}
