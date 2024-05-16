package provider

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"os"
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
	node        = "pve"
	ostemplate  = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"

	hostname = "wall-e"
	password = "garbageiscool"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("wall-e")),
					testCheckLXCPassword(&lxc, "root", "garbageiscool"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "wall-e"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "password", "garbageiscool"),
				),
			},
			{
				Config: providerConfig + `
resource "proxmox_lxc" "test" {
	node        = "pve"
	ostemplate  = "local:vztmpl/alpine-3.18-default_20230607_amd64.tar.xz"
	
	hostname = "m-o"
	password = "sunday_clothes"
}
`,
				Check: resource.ComposeTestCheckFunc(
					testCheckLXCExistsInPve(ctx, "proxmox_lxc.test", &lxc),
					testCheckLXCValuesInPve(&lxc, types.StringValue("pve"), types.Int64Value(100), types.StringValue("alpine"), types.StringValue("m-o")),
					testCheckLXCPassword(&lxc, "root", "sunday_clothes"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "node", "pve"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "hostname", "m-o"),
					resource.TestCheckResourceAttr("proxmox_lxc.test", "password", "sunday_clothes"),
				),
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
