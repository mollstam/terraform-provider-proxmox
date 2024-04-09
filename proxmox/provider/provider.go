package provider

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	pveapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	pschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const defaultTLSInsecure = false
const defaultTimeout = 60
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
	APITokenID     types.String `tfsdk:"api_token_id"`
	APITokenSecret types.String `tfsdk:"api_token_secret"`
	TLSInsecure    types.Bool   `tfsdk:"tls_insecure"`
	HTTPHeaders    types.String `tfsdk:"http_headers"`
	Timeout        types.Int64  `tfsdk:"timeout"`
	Debug          types.Bool   `tfsdk:"debug"`
}

func (p *proxmoxProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "proxmox"
	resp.Version = p.version
}

func (*proxmoxProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = pschema.Schema{
		Description: "Interact with Proxmox VE",
		Attributes: map[string]pschema.Attribute{
			"api_url": rschema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					URLValidator("you must specify a valid endpoint for the Proxmox Virtual Environment API (https://host.fqdn:8006/api2/json)"),
				},
				Description: "https://host.fqdn:8006/api2/json",
			},
			"api_token_id": rschema.StringAttribute{
				Optional:    true,
				Description: "API token ID prefixed by full User ID e.g. name@realm!token",
			},
			"api_token_secret": rschema.StringAttribute{
				Optional:    true,
				Description: "API token secret e.g. 3b5a972d-bdb2-4181-b8f2-e3cdb34b3b4f",
				Sensitive:   true,
			},
			"tls_insecure": rschema.BoolAttribute{
				Optional:    true,
				Default:     booldefault.StaticBool(defaultTLSInsecure),
				Computed:    true,
				Description: "By default, every TLS connection is verified to be secure. This option allows terraform to proceed and operate on servers considered insecure. For example if you're connecting to a remote host and you do not have the CA cert that issued the proxmox api url's certificate.",
			},
			"http_headers": rschema.StringAttribute{
				Optional:    true,
				Description: "Set custom http headers e.g. Key,Value,Key1,Value1",
			},
			"timeout": rschema.Int64Attribute{
				Optional:    true,
				Default:     int64default.StaticInt64(defaultTimeout),
				Computed:    true,
				Description: fmt.Sprintf("How many seconds to wait for tasks in Proxmox VE to complete, default is %d", defaultTimeout),
			},
			"debug": rschema.BoolAttribute{
				Optional:    true,
				Default:     booldefault.StaticBool(defaultDebug),
				Computed:    true,
				Description: "Enable or disable the verbose debug output from the API client",
			},
		},
	}
}

func (*proxmoxProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
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

	if config.APITokenID.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token_id"),
			"Unknown Proxmox VE API Token ID",
			"The provider cannot create the API client as api_token_id is set to an unknown configuration value. "+
				"Either target apply the source of the value first, set the value statically, or use the PVE_API_TOKEN_ID environment variable.",
		)
	}

	if config.APITokenSecret.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token_secret"),
			"Unknown Proxmox VE API Token Secret",
			"The provider cannot create the API client as api_token_secret is set to an unknown configuration value. "+
				"Either target apply the source of the value first, set the value statically, or use the PVE_API_TOKEN_SECRET environment variable.",
		)
	}

	if config.TLSInsecure.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("tls_insecure"),
			"Unknown Proxmox VE TLS Insecure",
			"The provider cannot create the API client as tls_insecure is set to an unknown configuration value. "+
				"Either target apply the source of the value first, set the value statically, or use the PVE_TLS_INSECURE environment variable.",
		)
	}

	if config.HTTPHeaders.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("http_headers"),
			"Unknown Proxmox VE HTTP Headers",
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

	apiURL := os.Getenv("PVE_API_URL")
	if !config.APIURL.IsNull() {
		apiURL = config.APIURL.ValueString()
	}

	apiTokenID := os.Getenv("PVE_API_TOKEN_ID")
	if !config.APITokenID.IsNull() {
		apiTokenID = config.APITokenID.ValueString()
	}

	apiTokenSecret := os.Getenv("PVE_API_TOKEN_SECRET")
	if !config.APITokenSecret.IsNull() {
		apiTokenSecret = config.APITokenSecret.ValueString()
	}

	tlsInsecure := GetenvOrDefaultBool("PVE_TLS_INSECURE", defaultTLSInsecure)
	if !config.TLSInsecure.IsNull() {
		tlsInsecure = config.TLSInsecure.ValueBool()
	}

	httpHeaders := os.Getenv("PVE_HTTP_HEADERS")
	if !config.HTTPHeaders.IsNull() {
		httpHeaders = config.HTTPHeaders.ValueString()
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

	if apiTokenID != "" && !strings.Contains(apiTokenID, "!") {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token_id"),
			"Malformed API Token ID",
			"Your API Token ID should contain a !, check your API credentials.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "proxmox_api_url", apiURL)
	tflog.Debug(ctx, "Creating Proxmox API client")

	tlsConf := &tls.Config{InsecureSkipVerify: true}
	if !tlsInsecure {
		tlsConf = nil
	}

	client, err := newProxmoxClient(
		apiURL,
		apiTokenID,
		apiTokenSecret,
		tlsConf,
		httpHeaders,
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

	minimumPermissions := []string{
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
	id := strings.Split(apiTokenID, "!")[0]
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
	sort.Strings(minimumPermissions)

	var permDiff []string
	for _, str2 := range minimumPermissions {
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

func (*proxmoxProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewVMResource,
	}
}

func (*proxmoxProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func newProxmoxClient(apiURL string,
	apiTokenID string,
	apiTokenSecret string,
	tlsConf *tls.Config,
	httpHeaders string,
	timeout int,
	debug bool) (*pveapi.Client, error) {
	var err error
	if apiTokenSecret == "" {
		err = errors.New("API token secret not provided, must exist")
	}

	if !strings.Contains(apiTokenID, "!") {
		err = errors.New("your API Token ID should contain a !, check your API credentials")
	}
	if err != nil {
		return nil, err
	}

	client, _ := pveapi.NewClient(apiURL, nil, httpHeaders, tlsConf, "", timeout)
	*pveapi.Debug = debug

	client.SetAPIToken(apiTokenID, apiTokenSecret)

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
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			panic(err)
		}

		return i
	}
	return dv
}
