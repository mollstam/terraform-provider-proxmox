package provider

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	pveapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/mollstam/terraform-provider-proxmox/proxmox/provider/testutil"
	"github.com/onsi/gomega"
)

func TestAccVMResource_CreateAndUpdate(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"
	description = "Waste Allocation Load Lifter: Earth-Class"

	memory = 32
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("wall-e"), types.StringValue("Waste Allocation Load Lifter: Earth-Class"), types.Int64Value(32)),
					testCheckVMStatusInPve(&vm, "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "vmid", "100"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "name", "wall-e"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "description", "Waste Allocation Load Lifter: Earth-Class"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "status", "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "memory", "32"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "m-o"
	description = "Microbe-Obliterator"

	memory = 40
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("m-o"), types.StringValue("Microbe-Obliterator"), types.Int64Value(40)),
					testCheckVMStatusInPve(&vm, "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "vmid", "100"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "name", "m-o"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "description", "Microbe-Obliterator"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "status", "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "memory", "40"),
				),
			},
		},
	})
}

func TestAccVMResource_CreateWithoutName_IsNullInState(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringNull(), types.StringNull(), types.Int64Value(16)),
					testCheckVMStatusInPve(&vm, "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "vmid", "100"),
					resource.TestCheckNoResourceAttr("proxmox_vm.test", "name"),
					resource.TestCheckNoResourceAttr("proxmox_vm.test", "description"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "status", "running"),
				),
			},
		},
	})
}

func TestAccVMResource_CreateAndUpdateStopped(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"
	status = "stopped"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMStatusInPve(&vm, "stopped"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"
	status = "running"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMStatusInPve(&vm, "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "status", "running"),
				),
			},
		},
	})
}

func TestAccVMResource_RefreshOutOfBandDestroyedVM_SucceedsWithNonEmptyPlan(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
				),
			},
			{
				RefreshState:       true,
				PreConfig:          destroyVMInPve(&vm),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccVMResource_ApplyOutOfBandDestroyedVM_IsRecreated(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
				),
			},
			{
				PreConfig: destroyVMInPve(&vm),
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
				),
			},
		},
	})
}

func TestAccVMResource_ApplyOutOfBandStoppedVM_IsStarted(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"
	status = "running"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMStatusInPve(&vm, "running"),
				),
			},
			{
				PreConfig: stopVMInPve(&vm),
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"
	status = "running"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMStatusInPve(&vm, "running"),
				),
			},
		},
	})
}

func TestAccVMResource_ApplyOutOfBandStartedVM_IsStopped(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"
	status = "stopped"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMStatusInPve(&vm, "stopped"),
				),
			},
			{
				PreConfig: startVMInPve(&vm),
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"
	status = "stopped"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMStatusInPve(&vm, "stopped"),
				),
			},
		},
	})
}

func TestAccVMResource_DestroyRunningVM(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"
}
`,
				Destroy: true,
			},
		},
	})
}

func TestAccVMResource_DestroyStoppedVM(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node   = "pve"
	name   = "wall-e"
	status = "stopped"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node   = "pve"
	name   = "wall-e"
	status = "stopped"
}
`,
				Destroy: true,
			},
		},
	})
}

func TestAccVMResource_UnconfigureVMID(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node   = "pve"
	name   = "wall-e"
	vmid   = 140
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(140), types.StringValue("wall-e"), types.StringNull(), types.Int64Value(16)),
					resource.TestCheckResourceAttr("proxmox_vm.test", "vmid", "140"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node   = "pve"
	name   = "wall-e"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(140), types.StringValue("wall-e"), types.StringNull(), types.Int64Value(16)),
					resource.TestCheckResourceAttr("proxmox_vm.test", "vmid", "140"),
				),
			},
		},
	})
}

func testCheckVMExistsInPve(ctx context.Context, n string, r *vmResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		vmid, err := strconv.ParseInt(rs.Primary.Attributes["vmid"], 10, 64)
		if err != nil {
			return err
		}

		err = UpdateResourceModelFromAPI(ctx, int(vmid), testutil.TestClient, r, VMStateEverything)
		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckVMValuesInPve(r *vmResourceModel, node basetypes.StringValue, vmid basetypes.Int64Value, name basetypes.StringValue, description basetypes.StringValue, memory basetypes.Int64Value) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		err := gomega.InterceptGomegaFailure(func() {
			gomega.Expect(r.Node).To(gomega.Equal(node))
			gomega.Expect(r.VMID).To(gomega.Equal(vmid))
			gomega.Expect(r.Name).To(gomega.Equal(name))
			gomega.Expect(r.Description).To(gomega.Equal(description))
			gomega.Expect(r.Memory).To(gomega.Equal(memory))
		})
		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckVMStatusInPve(r *vmResourceModel, status string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		err := gomega.InterceptGomegaFailure(func() {
			gomega.Expect(r.Status.IsUnknown()).To(gomega.BeFalseBecause("Status should be a known value"))
			gomega.Expect(r.Status.IsNull()).To(gomega.BeFalseBecause("Status should not be null"))
			gomega.Expect(r.Status.ValueString()).To(gomega.Equal(status))
		})
		if err != nil {
			return err
		}

		return nil
	}
}

func startVMInPve(r *vmResourceModel) func() {
	return func() {
		ref := pveapi.NewVmRef(int(r.VMID.ValueInt64()))
		ref.SetNode(r.Node.ValueString())

		_, err := testutil.TestClient.StartVm(ref)
		if err != nil {
			panic("Failed to start VM during test step: " + err.Error())
		}
	}
}

func stopVMInPve(r *vmResourceModel) func() {
	return func() {
		ref := pveapi.NewVmRef(int(r.VMID.ValueInt64()))
		ref.SetNode(r.Node.ValueString())

		_, err := testutil.TestClient.StopVm(ref)
		if err != nil {
			panic("Failed to stop VM during test step: " + err.Error())
		}
	}
}

func destroyVMInPve(r *vmResourceModel) func() {
	return func() {
		ref := pveapi.NewVmRef(int(r.VMID.ValueInt64()))
		ref.SetNode(r.Node.ValueString())

		_, err := testutil.TestClient.StopVm(ref)
		if err != nil {
			panic("Failed to stop VM before delete during test step: " + err.Error())
		}

		_, err = testutil.TestClient.DeleteVm(ref)
		if err != nil {
			panic("Failed to delete VM during test step: " + err.Error())
		}
	}
}
