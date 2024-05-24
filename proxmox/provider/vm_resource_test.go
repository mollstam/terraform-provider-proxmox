package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
	node        = "pve"
	name        = "wall-e"
	description = "Waste Allocation Load Lifter: Earth-Class"

	sockets = 2
	cores   = 2
	memory  = 32

	virtio0 = {
		media   = "disk"
		size    = 30
		storage = "local-lvm"
	}

	virtio1 = {
		media   = "disk"
		size    = 10
		storage = "local-lvm"
	}
	
	net = {
		name   = "eth0"
		bridge = "vmbr0"
	}
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("wall-e"), types.StringValue("Waste Allocation Load Lifter: Earth-Class"), types.Int64Value(2), types.Int64Value(2), types.Int64Value(32)),
					testCheckVMStorageValuesInPve(ctx, &vm, "virtio0", types.StringValue("local-lvm"), types.Int64Value(30)),
					testCheckVMStatusInPve(&vm, "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "vmid", "100"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "name", "wall-e"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "description", "Waste Allocation Load Lifter: Earth-Class"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "status", "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "sockets", "2"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "cores", "2"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "memory", "32"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "virtio0.media", "disk"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "virtio0.size", "30"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "virtio0.storage", "local-lvm"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "virtio0.format", "raw"),
					resource.TestCheckNoResourceAttr("proxmox_vm.test", "virtio2"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "memory", "32"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node        = "pve"
	name        = "m-o"
	description = "Microbe-Obliterator"

	sockets = 1
	cores   = 1
	memory  = 40

	virtio0 = {
		media   = "disk"
		size    = 30
		storage = "local-lvm"
	}

	virtio1 = {
		media   = "disk"
		size    = 10
		storage = "local-lvm"
	}
	
	net = {
		name   = "eth0"
		bridge = "vmbr0"
    }
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("m-o"), types.StringValue("Microbe-Obliterator"), types.Int64Value(1), types.Int64Value(1), types.Int64Value(40)),
					testCheckVMStatusInPve(&vm, "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "vmid", "100"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "name", "m-o"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "description", "Microbe-Obliterator"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "status", "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "sockets", "1"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "cores", "1"),
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
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringNull(), types.StringNull(), types.Int64Value(1), types.Int64Value(1), types.Int64Value(16)),
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

func TestAccVMResource_CreateTwoVMsWithoutVMID_GetSequentialIds(t *testing.T) {
	var vma, vmb vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test_a" {
	node        = "pve"
	name        = "wall-e"
	description = "Waste Allocation Load Lifter: Earth-Class"

	sockets = 2
	cores   = 2
	memory  = 32
}

resource "proxmox_vm" "test_b" {
	node        = "pve"
	name        = "eve"
	description = "Extraterrestrial Vegetation Evaluator"

	sockets = 2
	cores   = 2
	memory  = 32
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test_a", &vma),
					testCheckVMExistsInPve(ctx, "proxmox_vm.test_b", &vmb),
				),
			},
		},
	})
}

func TestAccVMResource_CreateTwoCloneVMsWithoutVMID_GetSequentialIds(t *testing.T) {
	var vma, vmb vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	template, err := createTemplateInPve(ctx, "Test-Template-01", 200, "pve", 16, 5)
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
resource "proxmox_vm" "test_a" {
	node        = "pve"
	name        = "wall-e"
	description = "Waste Allocation Load Lifter: Earth-Class"

	clone = 200

	sockets = 2
	cores   = 2
	memory  = 32
}

resource "proxmox_vm" "test_b" {
	node        = "pve"
	name        = "eve"
	description = "Extraterrestrial Vegetation Evaluator"

	clone = 200

	sockets = 2
	cores   = 2
	memory  = 32
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test_a", &vma),
					testCheckVMExistsInPve(ctx, "proxmox_vm.test_b", &vmb),
				),
			},
		},
	})
}

func TestAccVMResource_CreateTwoVMsWithSameVMID_CausesError(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test_a" {
	node        = "pve"
	name        = "wall-e"
	description = "Waste Allocation Load Lifter: Earth-Class"
	vmid        = 100

	sockets = 2
	cores   = 2
	memory  = 32
}

resource "proxmox_vm" "test_b" {
	node        = "pve"
	name        = "eve"
	description = "Extraterrestrial Vegetation Evaluator"
	vmid        = 100

	sockets = 2
	cores   = 2
	memory  = 32
}
`,
				ExpectError: regexp.MustCompile(`VM 100 already exists`),
			},
		},
	})
}

func TestAccVMResource_CreateWithAgent_IpCanBeRead(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"

	agent = true
	clone = 300

	memory = 2048

	virtio0 = {
		media   = "disk"
		size    = 20
		storage = "local-lvm"
	}
	
	net = {
		name   = "eth0"
		bridge = "vnet0"
		ip     = "dhcp"
    }
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("Copy-of-VM-agent-test-template"), types.StringNull(), types.Int64Value(1), types.Int64Value(1), types.Int64Value(2048)),
					testCheckVMStatusInPve(&vm, "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm.test", "status", "running"),
					resource.TestCheckResourceAttrWith("proxmox_vm.test", "ipv4_address", func(value string) error {
						prefix := "10.0.0."
						if !strings.HasPrefix(value, prefix) {
							return errors.New("Expected ipv4_address to start with " + prefix)
						}
						return nil
					}),
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

	sockets = 1
	cores = 4
	memory = 36
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
				),
			},
			{
				PreConfig: testutil.ComposeFunc(
					setVMMemoryInPve(&vm, 30),
					setVMSocketsInPve(&vm, 2),
					setVMCoresInPve(&vm, 2),
				),
				Config: providerConfig + `
resource "proxmox_vm" "test" {
	node = "pve"
	name = "wall-e"

	sockets = 1
	cores = 4
	memory = 36
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("wall-e"), types.StringNull(), types.Int64Value(1), types.Int64Value(4), types.Int64Value(36)),
					resource.TestCheckResourceAttr("proxmox_vm.test", "memory", "36"),
				),
			},
		},
	})
}

func TestAccVMResource_CreateCloneOfTemplate(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	template, err := createTemplateInPve(ctx, "Test-Template-01", 200, "pve", 16, 5)
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
	
	clone = "200"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test_clone", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("m-o"), types.StringValue("Microbe-Obliterator"), types.Int64Value(1), types.Int64Value(1), types.Int64Value(16)),
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
func TestAccVMResource_CreateCloneOfTemplateByName(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	template, err := createTemplateInPve(ctx, "Test-Template-01", 200, "pve", 16, 5)
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
	
	clone = "Test-Template-01"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test_clone", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("m-o"), types.StringValue("Microbe-Obliterator"), types.Int64Value(1), types.Int64Value(1), types.Int64Value(16)),
					testCheckVMStatusInPve(&vm, "running"),
					testCheckVMIsCloneOf(&vm, template),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "vmid", "100"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "name", "m-o"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "description", "Microbe-Obliterator"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "status", "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "clone", "Test-Template-01"),
				),
			},
		},
	})
}

func TestAccVMResource_CreateAndUpdateToClone_ShouldBeRecreatedAsClone(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	template, err := createTemplateInPve(ctx, "Test-Template-01", 200, "pve", 16, 5)
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
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("m-o"), types.StringValue("Microbe-Obliterator"), types.Int64Value(1), types.Int64Value(1), types.Int64Value(16)),
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

	clone = "200"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test_to_be_clone", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("m-o"), types.StringValue("Microbe-Obliterator"), types.Int64Value(1), types.Int64Value(1), types.Int64Value(16)),
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

func TestAccVMResource_CreateCloneAndUpdateToAnotherClone_ShouldBeRecreated(t *testing.T) {
	var vm vmResourceModel

	ctx := testutil.GetTestLoggingContext()

	template1, err := createTemplateInPve(ctx, "Test-Template-01", 200, "pve", 16, 5)
	if err != nil {
		t.Error("Error during setup: " + err.Error())
		return
	}
	cleanUpFunc1 := destroyVMInPve(template1)
	defer cleanUpFunc1()

	template2, err := createTemplateInPve(ctx, "Test-Template-02", 201, "pve", 16, 7)
	if err != nil {
		t.Error("Error during setup: " + err.Error())
		return
	}
	cleanUpFunc2 := destroyVMInPve(template2)
	defer cleanUpFunc2()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_vm" "test_clone" {
	node = "pve"
	name = "m-o"
	description = "Microbe-Obliterator"

	clone = "200"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test_clone", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("m-o"), types.StringValue("Microbe-Obliterator"), types.Int64Value(1), types.Int64Value(1), types.Int64Value(16)),
					testCheckVMStatusInPve(&vm, "running"),
					testCheckVMIsCloneOf(&vm, template1),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "vmid", "100"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "name", "m-o"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "description", "Microbe-Obliterator"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "status", "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "clone", "200"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_vm" "test_clone" {
	node = "pve"
	name = "m-o"
	description = "Microbe-Obliterator"

	clone = "201"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckVMExistsInPve(ctx, "proxmox_vm.test_clone", &vm),
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(100), types.StringValue("m-o"), types.StringValue("Microbe-Obliterator"), types.Int64Value(1), types.Int64Value(1), types.Int64Value(16)),
					testCheckVMStatusInPve(&vm, "running"),
					testCheckVMIsCloneOf(&vm, template2),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "vmid", "100"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "name", "m-o"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "description", "Microbe-Obliterator"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "status", "running"),
					resource.TestCheckResourceAttr("proxmox_vm.test_clone", "clone", "201"),
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
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(140), types.StringValue("wall-e"), types.StringNull(), types.Int64Value(1), types.Int64Value(1), types.Int64Value(16)),
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
					testCheckVMValuesInPve(&vm, types.StringValue("pve"), types.Int64Value(140), types.StringValue("wall-e"), types.StringNull(), types.Int64Value(1), types.Int64Value(1), types.Int64Value(16)),
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

		err = UpdateVMResourceModelFromAPI(ctx, int(vmid), testutil.TestClient, r, VMStateEverything)
		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckVMIsCloneOf(r *vmResourceModel, t *vmResourceModel) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		vmid := int(r.VMID.ValueInt64())
		vmr := pveapi.NewVmRef(vmid)
		config, err := pveapi.NewConfigQemuFromApi(vmr, testutil.TestClient)
		if err != nil {
			return err
		}
		volume, ok := config.QemuUnusedDisks[0]["file"].(string)
		if !ok {
			panic("Unable to read qemu disk file property as string")
		}
		re := regexp.MustCompile(`^(\d+)/`)
		m := re.FindStringSubmatch(volume)
		if m == nil {
			panic("Unable to parse template id from clone volume " + volume)
		}
		c, err := strconv.ParseInt(m[1], 10, 0)
		if err != nil {
			return err
		}
		cv := types.Int64Value(c)

		err = gomega.InterceptGomegaFailure(func() {
			gomega.Expect(cv).To(gomega.Equal(t.VMID))
		})
		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckVMValuesInPve(r *vmResourceModel, node basetypes.StringValue, vmid basetypes.Int64Value, name basetypes.StringValue, description basetypes.StringValue, sockets basetypes.Int64Value, cores basetypes.Int64Value, memory basetypes.Int64Value) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		err := gomega.InterceptGomegaFailure(func() {
			gomega.Expect(r.Node).To(gomega.Equal(node))
			gomega.Expect(r.VMID).To(gomega.Equal(vmid))
			gomega.Expect(r.Name).To(gomega.Equal(name))
			gomega.Expect(r.Description).To(gomega.Equal(description))
			gomega.Expect(r.Sockets).To(gomega.Equal(sockets))
			gomega.Expect(r.Cores).To(gomega.Equal(cores))
			gomega.Expect(r.Memory).To(gomega.Equal(memory))
		})
		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckVMStorageValuesInPve(ctx context.Context, r *vmResourceModel, endpoint string, storage basetypes.StringValue, size basetypes.Int64Value) resource.TestCheckFunc {
	re := regexp.MustCompile(`^(virtio)(\d+)`)
	return func(_ *terraform.State) error {
		err := gomega.InterceptGomegaFailure(func() {
			m := re.FindStringSubmatch(endpoint)
			if m == nil {
				panic("Unable to parse endpoint " + endpoint)
			}
			if m[1] == "virtio" && m[2] == "0" {
				gomega.Expect(r.Virtio0.IsNull()).To(gomega.BeFalseBecause("virtio0 should not be null"))

				var dm virtioModel
				diags := r.Virtio0.As(ctx, &dm, basetypes.ObjectAsOptions{})
				if diags.HasError() {
					panic("error when reading virtio0 from resource model")
				}
				gomega.Expect(dm.Storage).To(gomega.Equal(storage))
				gomega.Expect(dm.Size).To(gomega.Equal(size))
			}
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

func createTemplateInPve(ctx context.Context, name string, vmid int, node string, memory int, size int) (*vmResourceModel, error) {
	ref := pveapi.NewVmRef(vmid)
	ref.SetNode(node)

	config := pveapi.ConfigQemu{}
	config.Name = name
	config.Memory = memory

	config.Disks = &pveapi.QemuStorages{
		VirtIO: &pveapi.QemuVirtIODisks{
			Disk_0: &pveapi.QemuVirtIOStorage{
				Disk: &pveapi.QemuVirtIODisk{
					Storage:         "local",
					SizeInKibibytes: pveapi.QemuDiskSize(size * 1024 * 1024),
					Format:          pveapi.QemuDiskFormat_Raw,
				},
			},
		},
	}

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
	err = UpdateVMResourceModelFromAPI(ctx, vmid, testutil.TestClient, &vm, VMStateEverything)
	if err != nil {
		return nil, err
	}
	return &vm, nil
}

func setVMSocketsInPve(r *vmResourceModel, sockets int) func() {
	return func() {
		ref := pveapi.NewVmRef(int(r.VMID.ValueInt64()))
		ref.SetNode(r.Node.ValueString())

		config, err := pveapi.NewConfigQemuFromApi(ref, testutil.TestClient)
		if err != nil {
			panic("Unexpected error when test setting VM sockets, reading config from API resulted in error: " + err.Error())
		}
		config.QemuSockets = sockets
		rebootRequried, err := config.Update(false, ref, testutil.TestClient)
		if err != nil {
			panic("Unexpected error when test setting VM sockets, updating config in API resulted in error: " + err.Error())
		}

		if rebootRequried {
			_, err = testutil.TestClient.StopVm(ref)
			if err != nil {
				panic("Unexpected error when test setting VM sockets, stopping VM resulted in error: " + err.Error())
			}
			_, err = testutil.TestClient.StartVm(ref)
			if err != nil {
				panic("Unexpected error when test setting VM sockets, starting VM resulted in error: " + err.Error())
			}
		}
	}
}

func setVMCoresInPve(r *vmResourceModel, cores int) func() {
	return func() {
		ref := pveapi.NewVmRef(int(r.VMID.ValueInt64()))
		ref.SetNode(r.Node.ValueString())

		config, err := pveapi.NewConfigQemuFromApi(ref, testutil.TestClient)
		if err != nil {
			panic("Unexpected error when test setting VM cores, reading config from API resulted in error: " + err.Error())
		}
		config.QemuCores = cores
		rebootRequried, err := config.Update(false, ref, testutil.TestClient)
		if err != nil {
			panic("Unexpected error when test setting VM cores, updating config in API resulted in error: " + err.Error())
		}

		if rebootRequried {
			_, err = testutil.TestClient.StopVm(ref)
			if err != nil {
				panic("Unexpected error when test setting VM cores, stopping VM resulted in error: " + err.Error())
			}
			_, err = testutil.TestClient.StartVm(ref)
			if err != nil {
				panic("Unexpected error when test setting VM cores, starting VM resulted in error: " + err.Error())
			}
		}
	}
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
