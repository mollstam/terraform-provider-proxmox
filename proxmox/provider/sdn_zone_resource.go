package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	pveapi "github.com/mollstam/proxmox-api-go/proxmox"
)

var (
	_ resource.Resource                = &sdnZoneResource{}
	_ resource.ResourceWithConfigure   = &sdnZoneResource{}
	_ resource.ResourceWithImportState = &sdnZoneResource{}
)

const (
	sdnZoneTypeEvpn   string = "evpn"
	sdnZoneTypeFaucet string = "faucet"
	sdnZoneTypeQinq   string = "qinq"
	sdnZoneTypeSimple string = "simple"
	sdnZoneTypeVlan   string = "vlan"
	sdnZoneTypeVxlan  string = "vxlan"
)

func NewSDNZoneResource() resource.Resource {
	return &sdnZoneResource{}
}

type sdnZoneResource struct {
	client *pveapi.Client
}

type sdnZoneResourceModel struct {
	Zone   types.String `tfsdk:"zone"`
	Type   types.String `tfsdk:"type"`
	Bridge types.String `tfsdk:"bridge"`
}

func (*sdnZoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sdn_zone"
}

func (*sdnZoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This resource manages a Proxmox SDN Zone.",
		Attributes: map[string]schema.Attribute{
			"zone": schema.StringAttribute{
				Description: "The SDN zone object identifier.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.All(
						SDNZoneValidator("zone must be a valid name (letters and numbers) between 2 and 8 characters (inclusive)."),
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Description: "The SDN zone object identifier.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf([]string{sdnZoneTypeEvpn, sdnZoneTypeFaucet, sdnZoneTypeQinq, sdnZoneTypeSimple, sdnZoneTypeVlan, sdnZoneTypeVxlan}...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bridge": schema.StringAttribute{
				Description: "",
				Required:    true,
			},
		},
	}
}

func (r *sdnZoneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*pveapi.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data Type",
			fmt.Sprintf("Expected %T, got: %T. Please report this to the provider developers.", client, req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *sdnZoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sdnZoneResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &pveapi.ConfigSDNZone{}
	err := apiConfigFromSDNZoneResourceModel(ctx, &plan, config, false)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error constructing API struct from internal model",
			"This is a provider bug. Please report it to the developers.\n\n"+err.Error())
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("Creating SDN Zone from model: %+v", plan))

	err = config.CreateWithValidate(plan.Zone.ValueString(), r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating SDN Zone",
			"Could not create SDN Zone, unexpected error: "+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, "Created SDN Zone")

	// ensure Computed attributes get set, configured attributes should remain stable
	err = UpdateSDNZoneResourceModelFromAPI(ctx, config.Zone, r.client, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating SDN Zone",
			fmt.Sprintf("Could not read back state of created SDN Zone %d, unexpected error:"+err.Error(), plan.Zone.ValueString()),
		)
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Setting state after creating SDN Zone to: %+v", plan))
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *sdnZoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sdnZoneResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !state.Zone.IsUnknown() {
		tflog.Trace(ctx, fmt.Sprintf("Reading state for SDN Zone %s", state.Zone.ValueString()))

		err := UpdateSDNZoneResourceModelFromAPI(ctx, state.Zone.ValueString(), r.client, &state)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading SDN State",
				fmt.Sprintf("Could not read state of SDN %s, unexpected error:"+err.Error(), state.Zone.ValueString()),
			)
			return
		}
		tflog.Trace(ctx, fmt.Sprintf("Read state %+v", state))
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *sdnZoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sdnZoneResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state sdnZoneResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Updating SDN Zone with plan: %+v", plan))

	config := &pveapi.ConfigSDNZone{}
	err := apiConfigFromSDNZoneResourceModel(ctx, &plan, config, true)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error constructing API struct from internal model",
			"This is a provider bug. Please report it to the developers.\n\n"+err.Error())
		return
	}

	err = config.UpdateWithValidate(state.Zone.ValueString(), r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating SDN Zone",
			"Could not update SDN Zone, unexpected error: "+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("SDN Zone %s updated", plan.Zone.ValueString()))

	var newState sdnZoneResourceModel

	err = UpdateSDNZoneResourceModelFromAPI(ctx, plan.Zone.ValueString(), r.client, &newState)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating SDN Zone",
			"Could not read back updated SDN Zone, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Setting state after updating SDN Zone to: %+v", newState))
	diags = resp.State.Set(ctx, newState)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *sdnZoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sdnZoneResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	const deleteErrorSummary string = "Error Deleting SDN Zone"
	tflog.Trace(ctx, fmt.Sprintf("Deleting SDN Zone %s", state.Zone.ValueString()))

	err := r.client.DeleteSDNZone(state.Zone.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			deleteErrorSummary,
			"Could not delete SDN Zone, unexpected error: "+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("SDN Zone %s deleted", state.Zone.ValueString()))
}

func (*sdnZoneResource) ImportState(_ context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.AddError(
		"ImportState Not Yet Supported",
		"Importing existing SDN Zone state is not currently supported, PRs welcome. :-)",
	)
}

func UpdateSDNZoneResourceModelFromAPI(ctx context.Context, zone string, client *pveapi.Client, model *sdnZoneResourceModel) error {
	tflog.Trace(ctx, "Updating sdnZoneResourceModel from PVE API.", map[string]any{"zone": zone})

	var config *pveapi.ConfigSDNZone
	var err error

	config, err = pveapi.NewConfigSDNZoneFromApi(zone, client)
	if err != nil {
		return err
	}
	tflog.Trace(ctx, fmt.Sprintf(".. updated config: %+v", config))

	model.Zone = types.StringValue(config.Zone)
	model.Type = types.StringValue(config.Type)
	model.Bridge = types.StringValue(config.Bridge)

	tflog.Trace(ctx, fmt.Sprintf("Updated sdnZoneResourceModel from PVE API, model is now %+v", model), map[string]any{"vmid": zone})

	return nil
}

func apiConfigFromSDNZoneResourceModel(ctx context.Context, model *sdnZoneResourceModel, config *pveapi.ConfigSDNZone, update bool) error {
	config.Zone = model.Zone.ValueString()
	config.Bridge = model.Bridge.ValueString()

	if !update {
		config.Type = model.Type.ValueString()
	}

	return nil
}
