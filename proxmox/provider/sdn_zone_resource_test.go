package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/mollstam/terraform-provider-proxmox/proxmox/provider/testutil"
	"github.com/onsi/gomega"
)

func TestAccSDNZoneResource_CreateAndUpdate(t *testing.T) {
	var zone sdnZoneResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_sdn_zone" "test" {
	zone   = "test"
	type   = "vlan"
	bridge = "vmbr0"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckSDNZoneExistsInPve(ctx, "proxmox_sdn_zone.test", &zone),
					testCheckSDNZoneValuesInPve(&zone, types.StringValue("test"), types.StringValue("vlan"), types.StringValue("vmbr0")),
					resource.TestCheckResourceAttr("proxmox_sdn_zone.test", "zone", "test"),
					resource.TestCheckResourceAttr("proxmox_sdn_zone.test", "type", "vlan"),
					resource.TestCheckResourceAttr("proxmox_sdn_zone.test", "bridge", "vmbr0"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_sdn_zone" "test" {
	zone   = "test"
	type   = "vlan"
	bridge = "vmbr1"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckSDNZoneExistsInPve(ctx, "proxmox_sdn_zone.test", &zone),
					testCheckSDNZoneValuesInPve(&zone, types.StringValue("test"), types.StringValue("vlan"), types.StringValue("vmbr1")),
					resource.TestCheckResourceAttr("proxmox_sdn_zone.test", "zone", "test"),
					resource.TestCheckResourceAttr("proxmox_sdn_zone.test", "type", "vlan"),
					resource.TestCheckResourceAttr("proxmox_sdn_zone.test", "bridge", "vmbr1"),
				),
			},
		},
	})
}

func TestAccSDNZoneResource_ChangeNameRecreatesZone(t *testing.T) {
	var zone sdnZoneResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_sdn_zone" "test" {
	zone   = "test"
	type   = "vlan"
	bridge = "vmbr0"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckSDNZoneExistsInPve(ctx, "proxmox_sdn_zone.test", &zone),
					testCheckSDNZoneValuesInPve(&zone, types.StringValue("test"), types.StringValue("vlan"), types.StringValue("vmbr0")),
					resource.TestCheckResourceAttr("proxmox_sdn_zone.test", "zone", "test"),
					resource.TestCheckResourceAttr("proxmox_sdn_zone.test", "type", "vlan"),
					resource.TestCheckResourceAttr("proxmox_sdn_zone.test", "bridge", "vmbr0"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_sdn_zone" "test" {
	zone   = "renamed"
	type   = "vlan"
	bridge = "vmbr0"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckSDNZoneExistsInPve(ctx, "proxmox_sdn_zone.test", &zone),
					testCheckSDNZoneValuesInPve(&zone, types.StringValue("renamed"), types.StringValue("vlan"), types.StringValue("vmbr0")),
					resource.TestCheckResourceAttr("proxmox_sdn_zone.test", "zone", "renamed"),
					resource.TestCheckResourceAttr("proxmox_sdn_zone.test", "type", "vlan"),
					resource.TestCheckResourceAttr("proxmox_sdn_zone.test", "bridge", "vmbr0"),
				),
			},
		},
	})
}

func testCheckSDNZoneExistsInPve(ctx context.Context, n string, r *sdnZoneResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		zone := rs.Primary.Attributes["zone"]

		err := UpdateSDNZoneResourceModelFromAPI(ctx, zone, testutil.TestClient, r)
		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckSDNZoneValuesInPve(r *sdnZoneResourceModel, zone basetypes.StringValue, typ basetypes.StringValue, bridge basetypes.StringValue) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		err := gomega.InterceptGomegaFailure(func() {
			gomega.Expect(r.Zone).To(gomega.Equal(zone))
			gomega.Expect(r.Type).To(gomega.Equal(typ))
			gomega.Expect(r.Bridge).To(gomega.Equal(bridge))
		})
		if err != nil {
			return err
		}

		return nil
	}
}
