package provider

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	pveapi "github.com/mollstam/proxmox-api-go/proxmox"
	"github.com/mollstam/terraform-provider-proxmox/proxmox/provider/testutil"
	"github.com/onsi/gomega"
	"golang.org/x/net/websocket"
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
	node         = "pve"
	ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

	hostname = "wall-e"

	rootfs = {
		storage = "local-lvm"
		size    = "1G"
	}

	mp0 = {
		storage = "local-lvm"
		size    = "2G"
		mp      = "/mnt/foo"
	}

	net = {
		name   = "eth0"
		bridge = "vmbr0"
		ip     = "192.168.0.50/24"
		gw     = "192.168.0.1"
	}
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("wall-e"), types.BoolValue(false)),
					testCheckLXCRootfsValuesInPve(ctx, &lxc, types.StringValue("local-lvm"), types.StringValue("1G")),
					testCheckLXCMountpointValuesInPve(ctx, &lxc, 0, types.StringValue("local-lvm"), types.StringValue("2G"), types.StringValue("/mnt/foo")),
					testCheckLXCNetValuesInPve(ctx, &lxc, types.StringValue("eth0"), types.StringValue("vmbr0"), types.StringValue("192.168.0.50/24"), types.StringValue("192.168.0.1")),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "wall-e"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "rootfs.storage", "local-lvm"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "rootfs.size", "1G"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "mp0.storage", "local-lvm"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "mp0.size", "2G"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "mp0.mp", "/mnt/foo"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "net.name", "eth0"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "net.bridge", "vmbr0"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "net.ip", "192.168.0.50/24"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "net.gw", "192.168.0.1"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node         = "pve"
	ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"
	
	hostname = "m-o"

	rootfs = {
		storage = "local-lvm"
		size    = "2G"
	}

	mp0 = {
		storage = "local-lvm"
		size    = "3G"
		mp      = "/mnt/bar"
	}

	net = {
		name   = "eth0"
		bridge = "vmbr0"
		ip     = "dhcp"
	}
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("m-o"), types.BoolValue(false)),
					testCheckLXCRootfsValuesInPve(ctx, &lxc, types.StringValue("local-lvm"), types.StringValue("2G")),
					testCheckLXCMountpointValuesInPve(ctx, &lxc, 0, types.StringValue("local-lvm"), types.StringValue("3G"), types.StringValue("/mnt/bar")),
					testCheckLXCNetValuesInPve(ctx, &lxc, types.StringValue("eth0"), types.StringValue("vmbr0"), types.StringValue("dhcp"), types.StringNull()),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "m-o"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "rootfs.storage", "local-lvm"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "rootfs.size", "2G"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "mp0.storage", "local-lvm"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "mp0.size", "3G"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "mp0.mp", "/mnt/bar"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "net.name", "eth0"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "net.bridge", "vmbr0"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "net.ip", "dhcp"),
				),
			},
		},
	})
}

func TestAccLXCResource_CreateAndUpdatePassword(t *testing.T) {
	var lxc lxcResourceModel

	// changing password will recreate resource, hence its own test

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node         = "pve"
	ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

	hostname = "wall-e"
	password = "garbageiscool"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("wall-e"), types.BoolValue(false)),
					testCheckLXCPassword(&lxc, "root", "garbageiscool"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "wall-e"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "password", "garbageiscool"),
				),
			},
			{
				Config: providerConfig + `
			resource "proxmox_lxc" "test" {
				node         = "pve"
				ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

				hostname = "wall-e"
				password = "sunday_clothes"
			}
			`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("wall-e"), types.BoolValue(false)),
					testCheckLXCPassword(&lxc, "root", "sunday_clothes"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "wall-e"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "password", "sunday_clothes"),
				),
			},
		},
	})
}

func TestAccLXCResource_CreateAndUpdateSSHKeys(t *testing.T) {
	var lxc lxcResourceModel

	// changing ssh keys will recreate resource, hence its own test

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node         = "pve"
	ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

	hostname = "wall-e"
	ssh_public_keys = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQDfnHfHWUoXXyGPgjFLwH8SE3MozO90AAQI9A338Bm0Srn6SJkdlOyaQdLXbvkTv1UTLhiDUR2KIsyNALYzpq5wNWirbMa8+8eBElrQwNwDP1WNdRW63lL4C01mdqMavqLiYoycOJjpOe7EmDgnNixIPesjBwPx5tHELJdHiLrU6Q== walle@test"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("wall-e"), types.BoolValue(false)),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "wall-e"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "ssh_public_keys", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQDfnHfHWUoXXyGPgjFLwH8SE3MozO90AAQI9A338Bm0Srn6SJkdlOyaQdLXbvkTv1UTLhiDUR2KIsyNALYzpq5wNWirbMa8+8eBElrQwNwDP1WNdRW63lL4C01mdqMavqLiYoycOJjpOe7EmDgnNixIPesjBwPx5tHELJdHiLrU6Q== walle@test"),
				),
			},
			{
				Config: providerConfig + `
			resource "proxmox_lxc" "test" {
				node         = "pve"
				ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

				hostname = "wall-e"
				ssh_public_keys = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQCsPEwRz7XqtM0cIr9YtokMt6q7pt8Mz4h+nh+KC0WD163Puc5JZ0S9ZGcPX7fHObmXRquBZ1Ek4cBmi4SnY1V4/9bNWvDttFUVVhAwuLWJzf+pGyRnUZxl8VIwdLzGZvX6h0NWfwEIwjDyRZZW1VE/dlwyVTxUYwv2IhF8pdycNQ== walle@test"
			}
			`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("wall-e"), types.BoolValue(false)),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "wall-e"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "ssh_public_keys", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQCsPEwRz7XqtM0cIr9YtokMt6q7pt8Mz4h+nh+KC0WD163Puc5JZ0S9ZGcPX7fHObmXRquBZ1Ek4cBmi4SnY1V4/9bNWvDttFUVVhAwuLWJzf+pGyRnUZxl8VIwdLzGZvX6h0NWfwEIwjDyRZZW1VE/dlwyVTxUYwv2IhF8pdycNQ== walle@test"),
				),
			},
		},
	})
}

func TestAccLXCResource_CreateAndUpdateUnprivileged(t *testing.T) {
	var lxc lxcResourceModel

	// changing unprivileged will recreate resource, hence its own test

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node         = "pve"
	ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

	hostname     = "wall-e"
	unprivileged = true
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("wall-e"), types.BoolValue(true)),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "wall-e"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "unprivileged", "true"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node         = "pve"
	ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"
	
	hostname     = "wall-e"
	unprivileged = false
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("wall-e"), types.BoolValue(false)),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "wall-e"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "unprivileged", "false"),
				),
			},
		},
	})
}

func TestAccLXCResource_CreateTwoLXCs_GetSequentialIds(t *testing.T) {
	var lxca, lxcb lxcResourceModel

	ctx := testutil.GetTestLoggingContext()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test_a" {
	node         = "pve"
	ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

	hostname     = "wall-e"
}

resource "proxmox_lxc" "test_b" {
	node         = "pve"
	ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

	hostname     = "eve"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test_a", &lxca),
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test_b", &lxcb),
				),
			},
		},
	})
}

func TestAccLXCResource_CreateTwoLXCsWithSameVMID_CausesError(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test_a" {
	node         = "pve"
	ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"
	vmid         = 100

	hostname     = "wall-e"
}

resource "proxmox_lxc" "test_b" {
	node         = "pve"
	ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"
	vmid         = 100

	hostname     = "eve"
}
`,
				ExpectError: regexp.MustCompile(`CT 100 already exists`),
			},
		},
	})
}

func TestAccLXCResource_CreateMountpointWithInvalidSize_CausesError(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test_a" {
	node         = "pve"
	ostemplate   = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"
	vmid         = 100

	hostname     = "wall-e"

	mp0 = {
		storage = "local-lvm"
		size    = "2"
		mp      = "/mnt/foo"
	}
}
`,
				ExpectError: regexp.MustCompile(`size must be numbers only and a suffix of M or G`),
			},
		},
	})
}

func TestAccLXCResource_CreateAndUpdateStopped(t *testing.T) {
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

	status = "stopped"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCStatusInPve(&lxc, "stopped"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "status", "stopped"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node        = "pve"
	ostemplate  = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

	status = "running"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCStatusInPve(&lxc, "running"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "status", "running"),
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
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("wall-e"), types.BoolValue(false)),
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
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("CT100"), types.BoolValue(false)),
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
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("archlinux"), types.StringValue("CT100"), types.BoolValue(false)),
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

		err = UpdateLXCResourceModelFromAPI(ctx, int(vmid), testutil.TestClient, r, LXCStateEverything)
		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckLXCStatusInPve(r *lxcResourceModel, status string) resource.TestCheckFunc {
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

func testCheckLXCValuesInPve(r *lxcResourceModel, node basetypes.StringValue, vmid basetypes.Int64Value, ostype basetypes.StringValue, hostname basetypes.StringValue, unprivileged basetypes.BoolValue) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		err := gomega.InterceptGomegaFailure(func() {
			gomega.Expect(r.Node).To(gomega.Equal(node))
			gomega.Expect(r.VMID).To(gomega.Equal(vmid))
			gomega.Expect(r.Ostype).To(gomega.Equal(ostype))
			gomega.Expect(r.Hostname).To(gomega.Equal(hostname))
			gomega.Expect(r.Unprivileged).To(gomega.Equal(unprivileged))
		})
		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckLXCRootfsValuesInPve(ctx context.Context, r *lxcResourceModel, storage basetypes.StringValue, size basetypes.StringValue) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		err := gomega.InterceptGomegaFailure(func() {
			gomega.Expect(r.RootFs.IsNull()).To(gomega.BeFalseBecause("rootfs should not be null"))

			var dm rootfsModel
			diags := r.RootFs.As(ctx, &dm, basetypes.ObjectAsOptions{})
			if diags.HasError() {
				panic("error when reading rootfs from resource model")
			}
			gomega.Expect(dm.Storage).To(gomega.Equal(storage))
			gomega.Expect(dm.Size).To(gomega.Equal(size))
		})
		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckLXCMountpointValuesInPve(ctx context.Context, r *lxcResourceModel, index int, storage basetypes.StringValue, size basetypes.StringValue, mp basetypes.StringValue) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		err := gomega.InterceptGomegaFailure(func() {
			o := basetypes.ObjectValue{}
			if index == 0 {
				o = r.Mp0
			}
			gomega.Expect(o.IsNull()).To(gomega.BeFalseBecause("mp%d should not be null", index))

			var dm mountpointModel
			diags := o.As(ctx, &dm, basetypes.ObjectAsOptions{})
			if diags.HasError() {
				panic("error when reading mp from resource model")
			}
			gomega.Expect(dm.Storage).To(gomega.Equal(storage))
			gomega.Expect(dm.Size).To(gomega.Equal(size))
			gomega.Expect(dm.Mountpoint).To(gomega.Equal(mp))
		})
		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckLXCNetValuesInPve(ctx context.Context, r *lxcResourceModel, name basetypes.StringValue, bridge basetypes.StringValue, ip basetypes.StringValue, gw basetypes.StringValue) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		err := gomega.InterceptGomegaFailure(func() {
			gomega.Expect(r.Net.IsNull()).To(gomega.BeFalseBecause("net should not be null"))

			var dm lxcNetModel
			diags := r.Net.As(ctx, &dm, basetypes.ObjectAsOptions{})
			if diags.HasError() {
				panic("error when reading net from resource model")
			}
			gomega.Expect(dm.Name).To(gomega.Equal(name))
			gomega.Expect(dm.Bridge).To(gomega.Equal(bridge))
			gomega.Expect(dm.IP).To(gomega.Equal(ip))
			gomega.Expect(dm.Gateway).To(gomega.Equal(gw))
		})
		if err != nil {
			return err
		}

		return nil
	}
}

func testCheckLXCPassword(r *lxcResourceModel, user string, pw string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		vmr := pveapi.NewVmRef(int(r.VMID.ValueInt64()))
		vmr.SetVmType("lxc")
		vmr.SetNode(r.Node.ValueString())
		params := map[string]any{}
		res, err := testutil.TestClient.CreateTermProxy(vmr, params)
		if err != nil {
			return err
		}

		// this check got a bit out of hand..

		u, err := url.Parse(testutil.TestClient.ApiUrl)
		if err != nil {
			return err
		}
		u.Scheme = "wss"
		u = u.JoinPath("nodes", vmr.Node(), vmr.GetVmType(), strconv.Itoa(vmr.VmId()), "vncwebsocket")
		q := u.Query()
		q.Add("port", res["port"].(string))
		q.Add("vncticket", res["ticket"].(string))
		u.RawQuery = q.Encode()

		config, err := websocket.NewConfig(u.String(), "http://localhost/")
		if err != nil {
			return err
		}
		config.Header = testutil.TestClient.Header()
		config.Header["Host"] = []string{u.Host}
		config.TlsConfig = &tls.Config{InsecureSkipVerify: true}
		config.Protocol = append(config.Protocol, "binary")
		ws, err := websocket.DialConfig(config)
		if err != nil {
			return err
		}

		readChan := make(chan string)
		closeChan := make(chan bool)
		defer func() { closeChan <- true }()

		go func() {
			var o string
			msg := make([]byte, 1024)

			for {
				dl := time.Now().Add(time.Second * 5)
				err := ws.SetReadDeadline(dl)
				if err != nil {
					panic(err.Error())
				}

				n, err := ws.Read(msg)

				select {
				case <-closeChan:
					ws.Close()
					return
				default:
				}

				if err != nil {
					if errors.Is(err, os.ErrDeadlineExceeded) {
						continue
					}
					return
				}
				o = string(msg[:n])
				select {
				case readChan <- o:
				case <-time.After(time.Second * 5):
					panic("no one reading term socket within timeout")
				}
			}
		}()

		sendMessage := func(message string) error {
			_, err := ws.Write([]byte(message))
			return err
		}

		sendInput := func(message string) error {
			err := sendMessage(fmt.Sprintf("0:%d:%s\n", len(message), message))
			return err
		}

		var msg string
		var errRecvUntilTimeout error
		recvUntil := func(suffix string) error {
			var b strings.Builder
			for !strings.HasSuffix(msg, suffix) {
				select {
				case msg = <-readChan:
					_, err := b.WriteString(msg)
					if err != nil {
						panic(err.Error())
					}
				case <-time.After(time.Second * 5):
					errRecvUntilTimeout = fmt.Errorf("Timeout waiting for \"%s\" from term socket.\nvvvvv- Received data while waiting -vvvvv\n%s\n-----------------------------------------", suffix, b.String())
					return errRecvUntilTimeout
				}
			}
			return nil
		}

		s := fmt.Sprintf("root@pam:%s\n", res["ticket"])
		if err := sendMessage(s); err != nil {
			return err
		}

		if err := recvUntil("OK"); err != nil {
			return err
		}

		if err := recvUntil("\x1b[H\x1b[J"); err != nil {
			return err
		}

		// resize term
		if err := sendMessage("1:240:25:\n"); err != nil {
			return err
		}

		if err := recvUntil("login: "); err != nil {
			if !errors.Is(err, errRecvUntilTimeout) {
				return err
			}

			// if we didn't receive any login prompt, try sending a newline
			if err := sendInput("\n"); err != nil {
				return err
			}
			if err := recvUntil("login: "); err != nil {
				return err
			}
		}
		if err := sendInput(user + "\n"); err != nil {
			return err
		}

		if err := recvUntil("Password: "); err != nil {
			return err
		}
		if err := sendInput(pw + "\n"); err != nil {
			return err
		}

		// wait for a prompt as a means to deduce if we managed to sign in or not..
		if err := recvUntil(":~# \x1b[6n"); err != nil {
			return err
		}

		if err := sendInput("exit\n"); err != nil {
			return err
		}

		err = recvUntil("login: ")
		return err
	}
}
