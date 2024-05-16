package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	pveapi "github.com/mollstam/proxmox-api-go/proxmox"
)

var (
	_ resource.Resource                = &lxcResource{}
	_ resource.ResourceWithConfigure   = &lxcResource{}
	_ resource.ResourceWithImportState = &lxcResource{}
)

func NewLXCResource() resource.Resource {
	return &lxcResource{}
}

type lxcResource struct {
	client *pveapi.Client
}

type lxcResourceModel struct {
	Node types.String `tfsdk:"node"`
	VMID types.Int64  `tfsdk:"vmid"`

	Status types.String `tfsdk:"status"`

	Ostemplate   types.String `tfsdk:"ostemplate"`
	Unprivileged types.Bool   `tfsdk:"unprivileged"`
	Ostype       types.String `tfsdk:"ostype"`

	Hostname types.String `tfsdk:"hostname"`
	Password types.String `tfsdk:"password"`
}

type LXCStateMask uint8

const (
	LXCStateConfig LXCStateMask = 1 << iota
	LXCStateStatus

	LXCStateEverything LXCStateMask = 0xff
)

func (*lxcResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lxc"
}

func (*lxcResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This resource manages a Proxmox LXC.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Description: "The cluster node name.",
				Required:    true,
			},
			"vmid": schema.Int64Attribute{
				Description: "The (unique) ID of the VM.",
				Computed:    true,
				Optional:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Description: "LXC Container status.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(stateRunning),
				Validators: []validator.String{
					stringvalidator.OneOf([]string{stateStopped, stateRunning}...),
				},
			},
			"ostemplate": schema.StringAttribute{
				Description: "The OS template or backup file.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"unprivileged": schema.BoolAttribute{
				Description: "Makes the container run as unprivileged user. (Should not be modified manually.)",
				Computed:    true,
				Optional:    true,
				Default:     booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"ostype": schema.StringAttribute{
				Description: "OS type. This is used to setup configuration inside the container, and corresponds to lxc setup scripts in /usr/share/lxc/config/<ostype>.common.conf. Value 'unmanaged' can be used to skip OS specific setup.",
				Computed:    true,
				Optional:    true,
			},
			"hostname": schema.StringAttribute{
				Description: "Set a host name for the container.",
				Computed:    true,
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "Sets root password inside container.",
				Optional:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *lxcResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *lxcResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan lxcResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &pveapi.ConfigLxc{}
	err := apiConfigFromLXCResourceModel(ctx, &plan, config)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error constructing API struct from internal model",
			"This is a provider bug. Please report it to the developers.\n\n"+err.Error())
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("Creating LXC from model: %+v", plan))

	id, err := getIDToUse(plan.VMID, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Determining VM ID",
			"Unexpected error when getting next free VM ID from the API. If you can't solve this error please report it to the provider developers.\n\n"+err.Error())
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("Creating with VMID %d", id))
	vmr := pveapi.NewVmRef(id)
	vmr.SetNode(plan.Node.ValueString())
	vmr.SetVmType("lxc")

	err = config.CreateLxc(vmr, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating LXC",
			"Could not create LXC, unexpected error: "+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, "Created LXC")

	if plan.Status.ValueString() == stateRunning {
		tflog.Trace(ctx, "Starting LXC since status set to "+plan.Status.ValueString())
		_, err := r.client.StartVm(vmr)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Creating LXC",
				"Could not start LXC after creation, unexpected error: "+err.Error(),
			)
			return
		}
	}

	// populate Computed attributes
	plan.VMID = types.Int64Value(int64(vmr.VmId()))
	plan.Ostype = types.StringValue(config.OsType)
	if config.Hostname == "" {
		plan.Hostname = types.StringNull()
	} else {
		plan.Hostname = types.StringValue(config.Hostname)
	}
	plan.Unprivileged = types.BoolValue(config.Unprivileged)

	tflog.Trace(ctx, fmt.Sprintf("Setting state after creating LXC to: %+v", plan))
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *lxcResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state lxcResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !state.VMID.IsUnknown() {
		tflog.Trace(ctx, fmt.Sprintf("Reading state for LXC %d", state.VMID.ValueInt64()))

		vms, err := pveapi.ListGuests(r.client)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading LXC State",
				"Could not list VMs before reading, unexpected error:"+err.Error(),
			)
			return
		}

		vmExists := false
		for _, vm := range vms {
			if int64(vm.Id) == state.VMID.ValueInt64() {
				vmExists = true
				break
			}
		}

		if !vmExists {
			tflog.Trace(ctx, fmt.Sprintf("Can't read state of LXC %d, it doesn't exist", state.VMID.ValueInt64()))
			resp.State.RemoveResource(ctx)
			return
		}

		err = UpdateLXCResourceModelFromAPI(ctx, int(state.VMID.ValueInt64()), r.client, &state, LXCStateEverything)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading LXC State",
				fmt.Sprintf("Could not read state of LXC %d, unsepcted error:"+err.Error(), state.VMID.ValueInt64()),
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

func (r *lxcResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan lxcResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Updating LXC with plan: %+v", plan))

	config := &pveapi.ConfigLxc{}
	err := apiConfigFromLXCResourceModel(ctx, &plan, config)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error constructing API struct from internal model",
			"This is a provider bug. Please report it to the developers.\n\n"+err.Error())
		return
	}

	id, err := getIDToUse(plan.VMID, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Determining VM ID",
			"Unexpected error when getting next free VM ID from the API. If you can't solve this error please report it to the provider developers.\n\n"+err.Error())
		return
	}
	vmr := pveapi.NewVmRef(id)
	vmr.SetNode(plan.Node.ValueString())
	vmr.SetVmType("lxc")

	err = config.UpdateConfig(vmr, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating LXC",
			"Could not update LXC, unexpected error: "+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("LXC %d updated", id))

	reboot, err := pveapi.GuestHasPendingChanges(vmr, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating LXC",
			"Unable to determine if LXC needs reboot after updating it, unexpected error: "+err.Error(),
		)
		return
	}
	if reboot {
		// RebootVm (ie POST ../status/reboot) hangs and never completes, probably because we're testing on VMs with nothing installed
		tflog.Trace(ctx, fmt.Sprintf("Rebooting LXC %d...", id))

		_, err = r.client.StopVm(vmr)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating LXC",
				"Could not stop LXC as part of reboot after updating it, unexpected error: "+err.Error(),
			)
			return
		}

		_, err = r.client.StartVm(vmr)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating LXC",
				"Could not start LXC as part of reboot after updating it, unexpected error: "+err.Error(),
			)
			return
		}

		tflog.Trace(ctx, fmt.Sprintf("Rebooted LXC %d.", id))
	}

	var state lxcResourceModel

	// carry over values not part of PVE state
	state.Ostemplate = plan.Ostemplate
	state.Password = plan.Password

	err = UpdateLXCResourceModelFromAPI(ctx, id, r.client, &state, LXCStateEverything)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating LXC",
			"Could not read back updated LXC status, unexpected error: "+err.Error(),
		)
		return
	}

	if plan.Status.ValueString() != state.Status.ValueString() {
		switch plan.Status.ValueString() {
		case stateRunning:
			tflog.Trace(ctx, "Starting LXC since status in plan set to "+plan.Status.ValueString())
			_, err := r.client.StartVm(vmr)
			if err != nil {
				resp.Diagnostics.AddError(
					"Error Updating LXC",
					"Could not start LXC, unexpected error: "+err.Error(),
				)
				return
			}
		case stateStopped:
			tflog.Trace(ctx, "Starting LXC since status in plan set to "+plan.Status.ValueString())
			_, err := r.client.StopVm(vmr)
			if err != nil {
				resp.Diagnostics.AddError(
					"Error Updating LXC",
					"Could not stop LXC, unexpected error: "+err.Error(),
				)
				return
			}
		}
	}

	err = UpdateLXCResourceModelFromAPI(ctx, id, r.client, &state, LXCStateStatus)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating LXC",
			"Could not read back updated LXC status, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Setting state after updating LXC to: %+v", state))
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *lxcResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state lxcResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	const deleteErrorSummary string = "Error Deleting LXC"
	tflog.Trace(ctx, fmt.Sprintf("Deleting LXC %d", state.VMID.ValueInt64()))

	vms, err := pveapi.ListGuests(r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			deleteErrorSummary,
			"Could not list VMs before deleting, unepxected error:"+err.Error(),
		)
		return
	}

	vmExists := false
	for _, vm := range vms {
		if int64(vm.Id) == state.VMID.ValueInt64() {
			vmExists = true
			break
		}
	}

	if !vmExists {
		tflog.Trace(ctx, fmt.Sprintf("Can't delete LXM %d, doesn't exist", state.VMID.ValueInt64()))
		return
	}

	vmr := pveapi.NewVmRef(int(state.VMID.ValueInt64()))
	vmr.SetNode(state.Node.ValueString())
	vmr.SetVmType("lxc")

	vmState, err := r.client.GetVmState(vmr)
	if err != nil {
		resp.Diagnostics.AddError(
			deleteErrorSummary,
			"Could not get VM state before deleting, unexpected error: "+err.Error(),
		)
		return
	}
	var ok bool
	status, ok := vmState["status"].(string)
	if !ok {
		resp.Diagnostics.AddError(
			deleteErrorSummary,
			fmt.Sprintf("status field in VM state was not a string but %T", vmState["status"]),
		)
		return
	}

	if status == "running" {
		_, err = r.client.StopVm(vmr)
		if err != nil {
			resp.Diagnostics.AddError(
				deleteErrorSummary,
				"Could not stop VM before deleting, unexpected error: "+err.Error(),
			)
			return
		}
	}

	_, err = r.client.DeleteVm(vmr)
	if err != nil {
		resp.Diagnostics.AddError(
			deleteErrorSummary,
			"Could not delete VM, unexpected error: "+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("LXC %d deleted", vmr.VmId()))
}

func (*lxcResource) ImportState(_ context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.AddError(
		"ImportState Not Yet Supported",
		"Importing existing LXC state is not currently supported, PRs welcome. :-)",
	)
}

func UpdateLXCResourceModelFromAPI(ctx context.Context, vmid int, client *pveapi.Client, model *lxcResourceModel, sm LXCStateMask) error {
	vmr := pveapi.NewVmRef(vmid)

	tflog.Trace(ctx, "Updating lxcResourceModel from PVE API.", map[string]any{"vmid": vmid})

	var config *pveapi.ConfigLxc
	var err error

	if sm&LXCStateConfig != 0 {
		config, err = pveapi.NewConfigLxcFromApi(vmr, client)
		if err != nil {
			return err
		}
		tflog.Trace(ctx, fmt.Sprintf(".. updated config: %+v", config))
	}

	var status string
	if sm&LXCStateStatus != 0 {
		state, err := client.GetVmState(vmr)
		if err != nil {
			return err
		}
		var ok bool
		status, ok = state["status"].(string)
		if !ok {
			return fmt.Errorf("status field in VM state was not a string but %T", state["status"])
		}
		tflog.Trace(ctx, ".. updated status: "+status)
	}

	if sm&LXCStateConfig != 0 {
		model.Node = types.StringValue(vmr.Node())
		model.VMID = types.Int64Value(int64(vmr.VmId()))
		model.Ostype = types.StringValue(config.OsType)
		model.Hostname = types.StringValue(config.Hostname)
		model.Unprivileged = types.BoolValue(config.Unprivileged)
	}

	if sm&LXCStateStatus != 0 {
		model.Status = types.StringValue(status)
	}

	tflog.Trace(ctx, fmt.Sprintf("Updated lxcResourceModel from PVE API, model is now %+v", model), map[string]any{"vmid": vmid})

	return nil
}

func apiConfigFromLXCResourceModel(_ context.Context, model *lxcResourceModel, config *pveapi.ConfigLxc) error {
	// Node set via VmRef
	// VMID set via VmRef
	config.Ostemplate = model.Ostemplate.ValueString()

	if !model.Hostname.IsNull() && !model.Hostname.IsUnknown() {
		config.Hostname = model.Hostname.ValueString()
	}

	if !model.Password.IsNull() && !model.Password.IsUnknown() {
		config.Password = model.Password.ValueString()
	}

	if !model.Unprivileged.IsNull() && !model.Unprivileged.IsUnknown() {
		config.Unprivileged = model.Unprivileged.ValueBool()
	}

	return nil
}
