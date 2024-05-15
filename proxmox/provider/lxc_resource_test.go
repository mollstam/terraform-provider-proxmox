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

func TestAccLXCResource_CreateAndUpdate(t *testing.T) {
	var lxc lxcResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node        = "pve"
	ostemplate  = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

	hostname = "wall-e"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("wall-e")),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "wall-e"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node        = "pve"
	ostemplate  = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"
	
	hostname = "m-o"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("m-o")),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "m-o"),
				),
			},
		},
	})
}

func TestAccLXCResource_ApplyOutOfBandModified_IsReconciledToPlan(t *testing.T) {
	var lxc lxcResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node = "pve"
	ostemplate  = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

	hostname = "wall-e"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
				),
			},
			{
				PreConfig: testutil.ComposeFunc(
					setLXCHostnameInPve(&lxc, "m-o"),
				),
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node = "pve"
	ostemplate  = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

	hostname = "wall-e"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("wall-e")),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "wall-e"),
				),
			},
		},
	})
}

func TestAccLXCResource_ChangeOsTemplateWillRecreateContainer(t *testing.T) {
	var lxc lxcResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node        = "pve"
	ostemplate  = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("CT100")),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node        = "pve"
	ostemplate  = "local:vztmpl/archlinux-base_20230608-1_amd64.tar.zst"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("archlinux"), types.StringValue("CT100")),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
				),
			},
		},
	})
}

func setLXCHostnameInPve(r *lxcResourceModel, hostname string) func() {
	return func() {
		ref := pveapi.NewVmRef(int(r.VMID.ValueInt64()))
		ref.SetNode(r.Node.ValueString())

		config, err := pveapi.NewConfigLxcFromApi(ref, testutil.TestClient)
		if err != nil {
			panic("Unexpected error when test setting LXC hostname, reading config from API resulted in error: " + err.Error())
		}
		config.Hostname = hostname
		err = config.UpdateConfig(ref, testutil.TestClient)
		if err != nil {
			panic("Unexpected error when test setting LXC hostname, updating config in API resulted in error: " + err.Error())
		}
	}
}

func testCheckLXCExistsInPve(ctx context.Context, n string, r *lxcResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		vmid, err := strconv.ParseInt(rs.Primary.Attributes["vmid"], 10, 64)
		if err != nil {
			return err
		}

		err = UpdateLXCResourceModelFromAPI(ctx, int(vmid), testutil.TestClient, r)
		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckLXCValuesInPve(r *lxcResourceModel, node basetypes.StringValue, vmid basetypes.Int64Value, ostype basetypes.StringValue, hostname basetypes.StringValue) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		err := gomega.InterceptGomegaFailure(func() {
			gomega.Expect(r.Node).To(gomega.Equal(node))
			gomega.Expect(r.VMID).To(gomega.Equal(vmid))
			gomega.Expect(r.Ostype).To(gomega.Equal(ostype))
			gomega.Expect(r.Hostname).To(gomega.Equal(hostname))
		})
		if err != nil {
			return err
		}

		return nil
	}
}
