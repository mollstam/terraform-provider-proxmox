package provider

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	pveapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const defaultTLSInsecure = false
const defaultTimeout = 10
const defaultDebug = false

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &proxmoxProvider{
			version: version,
		}
	}
}

type proxmoxProvider struct {
	version string
}

type proxmoxProviderModel struct {
	APIURL         types.String `tfsdk:"api_url"`
	APITokenId     types.String `tfsdk:"api_token_id"`
	APITokenSecret types.String `tfsdk:"api_token_secret"`
	TLSInsecure    types.Bool   `tfsdk:"tls_insecure"`
	HTTPHeaders    types.String `tfsdk:"http_headers"`
	Timeout        types.Int64  `tfsdk:"timeout"`
	Debug          types.Bool   `tfsdk:"debug"`
}

func (p *proxmoxProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "proxmox"
	resp.Version = p.version
}

func (p *proxmoxProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Interact with Proxmox VE",
		Attributes: map[string]schema.Attribute{
			"api_url": resSchema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					URLValidator("you must specify a valid endpoint for the Proxmox Virtual Environment API (https://host.fqdn:8006/api2/json)"),
				},
				Description: "https://host.fqdn:8006/api2/json",
			},
			"api_token_id": resSchema.StringAttribute{
				Optional:    true,
				Description: "API TokenID e.g. root@pam!mytesttoken",
			},
			"api_token_secret": resSchema.StringAttribute{
				Optional:    true,
				Description: "The secret uuid corresponding to a TokenID",
				Sensitive:   true,
			},
			"tls_insecure": resSchema.BoolAttribute{
				Optional:    true,
				Default:     booldefault.StaticBool(defaultTLSInsecure),
				Computed:    true,
				Description: "By default, every TLS connection is verified to be secure. This option allows terraform to proceed and operate on servers considered insecure. For example if you're connecting to a remote host and you do not have the CA cert that issued the proxmox api url's certificate.",
			},
			"http_headers": resSchema.StringAttribute{
				Optional:    true,
				Description: "Set custom http headers e.g. Key,Value,Key1,Value1",
			},
			"timeout": resSchema.Int64Attribute{
				Optional:    true,
				Default:     int64default.StaticInt64(defaultTimeout),
				Computed:    true,
				Description: "How many seconds to wait for operations for both provider and api-client, default is 20m",
			},
			"debug": resSchema.BoolAttribute{
				Optional:    true,
				Default:     booldefault.StaticBool(defaultDebug),
				Computed:    true,
				Description: "Enable or disable the verbose debug output from the API client",
			},
		},
	}
}

func (p *proxmoxProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Debug(ctx, "Configuring Proxmox VE provider")

	var config proxmoxProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.APIURL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_url"),
			"Unknown Proxmox VE API URL",
			"The provider cannot create the API client as api_url is set to an unknown configuration value. "+
				"Either target apply the source of the value first, set the value statically, or use the PVE_API_URL environment variable.",
		)
	}

	if config.APITokenId.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token_id"),
			"Unknown Proxmox VE APITokenId",
			"The provider cannot create the API client as api_token_id is set to an unknown configuration value. "+
				"Either target apply the source of the value first, set the value statically, or use the PVE_API_TOKEN_ID environment variable.",
		)
	}

	if config.APITokenSecret.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token_secret"),
			"Unknown Proxmox VE APITokenSecret",
			"The provider cannot create the API client as api_token_secret is set to an unknown configuration value. "+
				"Either target apply the source of the value first, set the value statically, or use the PVE_API_TOKEN_SECRET environment variable.",
		)
	}

	if config.TLSInsecure.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("tls_insecure"),
			"Unknown Proxmox VE TLSInsecure",
			"The provider cannot create the API client as tls_insecure is set to an unknown configuration value. "+
				"Either target apply the source of the value first, set the value statically, or use the PVE_TLS_INSECURE environment variable.",
		)
	}

	if config.HTTPHeaders.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("http_headers"),
			"Unknown Proxmox VE HTTPHeaders",
			"The provider cannot create the API client as http_headers is set to an unknown configuration value. "+
				"Either target apply the source of the value first, set the value statically, or use the PVE_HTTP_HEADERS environment variable.",
		)
	}

	if config.Timeout.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("timeout"),
			"Unknown Proxmox VE Timeout",
			"The provider cannot create the API client as timeout is set to an unknown configuration value. "+
				"Either target apply the source of the value first, set the value statically, or use the PVE_TIMEOUT environment variable.",
		)
	}

	if config.Debug.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("debug"),
			"Unknown Proxmox VE Debug",
			"The provider cannot create the API client as debug is set to an unknown configuration value. "+
				"Either target apply the source of the value first, set the value statically, or use the PVE_DEBUG environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	api_url := os.Getenv("PVE_API_URL")
	if !config.APIURL.IsNull() {
		api_url = config.APIURL.ValueString()
	}

	api_token_id := os.Getenv("PVE_API_TOKEN_ID")
	if !config.APITokenId.IsNull() {
		api_token_id = config.APITokenId.ValueString()
	}

	api_token_secret := os.Getenv("PVE_API_TOKEN_SECRET")
	if !config.APITokenSecret.IsNull() {
		api_token_secret = config.APITokenSecret.ValueString()
	}

	tls_insecure := GetenvOrDefaultBool("PVE_TLS_INSECURE", defaultTLSInsecure)
	if !config.TLSInsecure.IsNull() {
		tls_insecure = config.TLSInsecure.ValueBool()
	}

	http_headers := os.Getenv("PVE_HTTP_HEADERS")
	if !config.HTTPHeaders.IsNull() {
		http_headers = config.HTTPHeaders.ValueString()
	}

	timeout := GetenvOrDefaultInt64("PVE_TIMEOUT", defaultTimeout)
	if !config.Timeout.IsNull() {
		timeout = config.Timeout.ValueInt64()
	}
	if timeout <= 0 {
		resp.Diagnostics.AddAttributeError(
			path.Root("timeout"),
			"Invalid Timeout",
			"Timeout must be greater than 0 (else all tasks will immediately time out)",
		)
	}

	debug := GetenvOrDefaultBool("PVE_DEBUG", defaultDebug)
	if !config.Debug.IsNull() {
		debug = config.Debug.ValueBool()
	}

	if api_token_id != "" && !strings.Contains(api_token_id, "!") {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token_id"),
			"Malformed API Token ID",
			"Your API Token ID should contain a !, check your API credentials.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "proxmox_api_url", api_url)
	tflog.Debug(ctx, "Creating Proxmox API client")

	client, err := newProxmoxClient(
		api_url,
		api_token_id,
		api_token_secret,
		tls_insecure,
		http_headers,
		int(timeout),
		debug)

	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create API client",
			"Unexpected error when creating the Proxmox API client, if not clear please contact the provider developers.\n\n"+err.Error())
		return
	}

	_, err = client.GetVersion()
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create API client",
			"Unexpected error when creating the Proxmox API client, sanity check failed while checking /version, make sure the API endpoint is correct.\n\n"+err.Error(),
		)
		return
	}

	//permission check
	minimum_permissions := []string{
		"Datastore.AllocateSpace",
		"Datastore.Audit",
		"Pool.Allocate",
		"Sys.Audit",
		"Sys.Console",
		"Sys.Modify",
		"VM.Allocate",
		"VM.Audit",
		"VM.Clone",
		"VM.Config.CDROM",
		"VM.Config.Cloudinit",
		"VM.Config.CPU",
		"VM.Config.Disk",
		"VM.Config.HWType",
		"VM.Config.Memory",
		"VM.Config.Network",
		"VM.Config.Options",
		"VM.Migrate",
		"VM.Monitor",
		"VM.PowerMgmt",
	}
	id := strings.Split(api_token_id, "!")[0]
	userID, err := pveapi.NewUserID(id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create API client",
			"Unexpected error when creating UserID object for the Proxmox API client, if not clear please contact the provider developers.\n\n"+err.Error())
		return
	}
	permlist, err := client.GetUserPermissions(userID, "/")
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create API client",
			"Unexpected error when checking API user permissions, if not clear please contact the provider developers.\n\n"+err.Error())
		return
	}
	sort.Strings(permlist)
	sort.Strings(minimum_permissions)

	var permDiff []string
	for _, str2 := range minimum_permissions {
		found := false
		for _, str1 := range permlist {
			if str2 == str1 {
				found = true
				break
			}
		}
		if !found {
			permDiff = append(permDiff, str2)
		}
	}
	if len(permDiff) != 0 {
		resp.Diagnostics.AddError(
			"Failed to create API client",
			fmt.Sprintf("Permissions for user/token %s are not sufficient, please provide also the following permissions that are missing: %v", userID.ToString(), permDiff))
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client

	tflog.Debug(ctx, "Configured Proxmox VE provider", map[string]any{"success": true})
}

func (p *proxmoxProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewVmQemuResource,
		/*func() resource.Resource {
			return &resourceLxc{} // proxmox_lxc
		},*/
		/*func() resource.Resource {
			return &resourceLxcDisk{} // proxmox_lxc_disk
		},
		func() resource.Resource {
			return &resourcePool{} // proxmox_pool
		},
		func() resource.Resource {
			return &resourceCloudInitDisk{} // proxmox_cloud_init_disk
		},
		func() resource.Resource {
			return &resourceStorageIso{} // proxmox_storage_iso
		},*/
	}
}

func (p *proxmoxProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		/*func() datasource.DataSource {
			return &DataHAGroup{} // proxmox_ha_groups
		},*/
	}
}

func newProxmoxClient(api_url string,
	api_token_id string,
	api_token_secret string,
	tls_insecure bool,
	http_headers string,
	timeout int,
	debug bool) (*pveapi.Client, error) {

	tlsconf := &tls.Config{InsecureSkipVerify: true}
	if !tls_insecure {
		tlsconf = nil
	}

	var err error
	if api_token_secret == "" {
		err = fmt.Errorf("API token secret not provided, must exist")
	}

	if !strings.Contains(api_token_id, "!") {
		err = fmt.Errorf("your API Token ID should contain a !, check your API credentials")
	}
	if err != nil {
		return nil, err
	}

	client, _ := pveapi.NewClient(api_url, nil, http_headers, tlsconf, "", timeout)
	*pveapi.Debug = debug

	client.SetAPIToken(api_token_id, api_token_secret)

	return client, nil
}

func GetenvOrDefaultBool(k string, dv bool) bool {
	if v := os.Getenv(k); v != "" {
		return v != "0" && v != "false" // anything else is truthy?
	}
	return dv
}

func GetenvOrDefaultInt64(k string, dv int64) int64 {
	if v := os.Getenv(k); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err != nil {
			panic(err)
		} else {
			return i
		}
	}
	return dv
}
