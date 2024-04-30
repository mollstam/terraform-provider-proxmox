package provider

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	pveapi "github.com/mollstam/proxmox-api-go/proxmox"
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

	agent = true
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
					resource.TestCheckResourceAttr("proxmox_vm.test", "agent", "true"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "memory", "32"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "m-o"
	description = "Microbe-Obliterator"

	agent = false
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
					resource.TestCheckResourceAttr("proxmox_vm.test", "agent", "false"),
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

func TestAccVMResource_ApplyOutOfBandModified_IsReconciledToPlan(t *testing.T) {
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

	memory = 36
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
				),
			},
			{
				PreConfig: setVMMemoryInPve(&vm, 30),
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"

	memory = 36
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("wall-e"), types.StringNull(), types.Int64Value(36)),
					resource.TestCheckResourceAttr("proxmox_vm.test", "memory", "36"),
				),
			},
		},
	})
}

func TestAccVMResource_CreateCloneOfTemplate(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	// we're using memory as a unique identifier here, expecting clone to have same non-default memory setting
	template, err := createTemplateInPve(ctx, 200, "pve", 22)
	if err != nil {
		t.Error("Error during setup: " + err.Error())
		return
	}
	cleanUpFunc := destroyVMInPve(template)
	defer cleanUpFunc()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test_clone" {
	node = "pve"
	name = "m-o"
	description = "Microbe-Obliterator"
	
	clone = 200
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test_clone", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("m-o"), types.StringValue("Microbe-Obliterator"), types.Int64Value(16)),
					testCheckVMStatusInPve(&vm, "running"),
					testCheckVMIsCloneOf(&vm, template),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "vmid", "100"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "name", "m-o"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "description", "Microbe-Obliterator"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "status", "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "clone", "200"),
				),
			},
		},
	})
}

func TestAccVMResource_CreateAndUpdateToClone_ShouldBeRecreatedAsClone(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	// we're using memory as a unique identifier here, expecting clone to have same non-default memory setting
	template, err := createTemplateInPve(ctx, 200, "pve", 32)
	if err != nil {
		t.Error("Error during setup: " + err.Error())
		return
	}
	cleanUpFunc := destroyVMInPve(template)
	defer cleanUpFunc()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test_to_be_clone" {
	node = "pve"
	name = "m-o"
	description = "Microbe-Obliterator"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test_to_be_clone", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("m-o"), types.StringValue("Microbe-Obliterator"), types.Int64Value(16)),
					testCheckVMStatusInPve(&vm, "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test_to_be_clone", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm.test_to_be_clone", "vmid", "100"),
					resource.TestCheckResourceAttr("proxmox_vm.test_to_be_clone", "name", "m-o"),
					resource.TestCheckResourceAttr("proxmox_vm.test_to_be_clone", "description", "Microbe-Obliterator"),
					resource.TestCheckResourceAttr("proxmox_vm.test_to_be_clone", "status", "running"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_vm" "test_to_be_clone" {
	node = "pve"
	name = "m-o"
	description = "Microbe-Obliterator"

	clone = 200
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test_to_be_clone", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("m-o"), types.StringValue("Microbe-Obliterator"), types.Int64Value(16)),
					testCheckVMStatusInPve(&vm, "running"),
					testCheckVMIsCloneOf(&vm, template),
					resource.TestCheckResourceAttr("proxmox_vm.test_to_be_clone", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm.test_to_be_clone", "vmid", "100"),
					resource.TestCheckResourceAttr("proxmox_vm.test_to_be_clone", "name", "m-o"),
					resource.TestCheckResourceAttr("proxmox_vm.test_to_be_clone", "description", "Microbe-Obliterator"),
					resource.TestCheckResourceAttr("proxmox_vm.test_to_be_clone", "status", "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test_to_be_clone", "clone", "200"),
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

func testCheckVMIsCloneOf(_ *vmResourceModel, _ *vmResourceModel) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		// We have no way of currently verifying that a running VM is a clone
		// When we have implemented support for storage that'll probably change
		return nil

		/* err := gomega.InterceptGomegaFailure(func() {
			gomega.Expect(r.Node).To(gomega.Equal(t.Node))
			gomega.Expect(r.VMID).To(gomega.Not(gomega.Equal(t.VMID)))
			gomega.Expect(r.Memory).To(gomega.Equal(t.Memory))
		})
		if err != nil {
			return err
		}

		return nil */
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

func createTemplateInPve(ctx context.Context, vmid int, node string, memory int) (*vmResourceModel, error) {
	ref := pveapi.NewVmRef(vmid)
	ref.SetNode(node)

	config := pveapi.ConfigQemu{}
	config.Memory = memory
	err := config.Create(ref, testutil.TestClient)
	if err != nil {
		return nil, err
	}

	_, err = testutil.TestClient.StopVm(ref)
	if err != nil {
		return nil, err
	}

	err = testutil.TestClient.CreateTemplate(ref)
	if err != nil {
		return nil, err
	}

	var vm vmResourceModel
	err = UpdateResourceModelFromAPI(ctx, vmid, testutil.TestClient, &vm, VMStateEverything)
	if err != nil {
		return nil, err
	}
	return &vm, nil
}

func setVMMemoryInPve(r *vmResourceModel, memory int) func() {
	return func() {
		ref := pveapi.NewVmRef(int(r.VMID.ValueInt64()))
		ref.SetNode(r.Node.ValueString())

		config, err := pveapi.NewConfigQemuFromApi(ref, testutil.TestClient)
		if err != nil {
			panic("Unexpected error when test setting VM memory, reading config from API resulted in error: " + err.Error())
		}
		config.Memory = memory
		rebootRequried, err := config.Update(false, ref, testutil.TestClient)
		if err != nil {
			panic("Unexpected error when test setting VM memory, updating config in API resulted in error: " + err.Error())
		}

		if rebootRequried {
			_, err = testutil.TestClient.StopVm(ref)
			if err != nil {
				panic("Unexpected error when test setting VM memory, stopping VM resulted in error: " + err.Error())
			}
			_, err = testutil.TestClient.StartVm(ref)
			if err != nil {
				panic("Unexpected error when test setting VM memory, starting VM resulted in error: " + err.Error())
			}
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
