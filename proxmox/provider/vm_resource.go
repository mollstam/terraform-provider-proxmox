package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	pveapi "github.com/mollstam/proxmox-api-go/proxmox"
)

var (
	_ resource.Resource                = &vmResource{}
	_ resource.ResourceWithConfigure   = &vmResource{}
	_ resource.ResourceWithImportState = &vmResource{}
)

const (
	stateRunning string = "running"
	stateStopped string = "stopped"
)

func NewVMResource() resource.Resource {
	return &vmResource{}
}

type vmResource struct {
	client *pveapi.Client
}

type vmResourceModel struct {
	Node        types.String `tfsdk:"node"`
	VMID        types.Int64  `tfsdk:"vmid"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`

	Status types.String `tfsdk:"status"`

	Clone types.Int64 `tfsdk:"clone"`

	Memory types.Int64 `tfsdk:"memory"`
}

type VMStateMask uint8

const (
	VMStateConfig VMStateMask = 1 << iota
	VMStateStatus

	VMStateEverything VMStateMask = 0xff
)

func (*vmResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

func (*vmResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This resource manages a Proxmox VM.",
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
			"name": schema.StringAttribute{
				Description: "Set a name for the VM. Only used on the configuration web interface.",
				Optional:    true,
			},
			"description": schema.StringAttribute{
				Description: "Description for the VM. Shown in the web-interface VM's summary. This is saved as comment inside the configuration file.",
				Optional:    true,
			},
			"status": schema.StringAttribute{
				Description: "QEMU process status.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(stateRunning),
				Validators: []validator.String{
					stringvalidator.OneOf([]string{stateStopped, stateRunning}...),
				},
			},
			"memory": schema.Int64Attribute{
				Description: "Memory in MB",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(16),
			},
			"clone": schema.Int64Attribute{
				Description: "Create a full clone of virtual machine/template with this VMID.",
				Optional:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplaceIfConfigured(),
				},
			},
		},
	}
}

func (r *vmResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *vmResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vmResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &pveapi.ConfigQemu{}
	apiConfigFromResourceModel(&plan, config)
	tflog.Trace(ctx, fmt.Sprintf("Creating VM from model: %+v", plan))

	id, err := getIDToUse(&plan, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Determining VM ID",
			"Unexpected error when getting next free VM ID from the API. If you can't solve this error please report it to the provider developers.\n\n"+err.Error())
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("Creating with VMID %d", id))
	vmr := pveapi.NewVmRef(id)
	vmr.SetNode(plan.Node.ValueString())

	if plan.Clone.IsNull() {
		err = config.Create(vmr, r.client)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Creating VM",
				"Could not create VM, unexpected error: "+err.Error(),
			)
			return
		}
		tflog.Trace(ctx, "Created VM")
	} else {
		srcvmr := pveapi.NewVmRef(int(plan.Clone.ValueInt64()))
		srcvmr.SetNode(plan.Node.ValueString())
		err = config.CloneVm(srcvmr, vmr, r.client)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Creating VM",
				"Could not clone VM, unexpected error: "+err.Error(),
			)
			return
		}
		tflog.Trace(ctx, "Created VM by cloning")

		// would be great if the API client read description from config and sent it along the clone request
		// .. until then, set it manually
		requiresReboot, err := config.Update(false, vmr, r.client)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Creating VM",
				"Could not update VM after cloning, unexpected error: "+err.Error(),
			)
			return
		}

		if requiresReboot {
			_, err = r.client.StopVm(vmr)
			if err != nil {
				resp.Diagnostics.AddError(
					"Error Creating VM",
					"Could not stop VM while rebooting after clone, unexpected error: "+err.Error(),
				)
				return
			}
			_, err = r.client.StartVm(vmr)
			if err != nil {
				resp.Diagnostics.AddError(
					"Error Creating VM",
					"Could not start VM while rebooting after clone, unexpected error: "+err.Error(),
				)
				return
			}
		}
	}

	if plan.Status.ValueString() == stateRunning {
		tflog.Trace(ctx, "Starting VM since status set to "+plan.Status.ValueString())
		_, err := r.client.StartVm(vmr)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Creating VM",
				"Could not start VM after creation, unexpected error: "+err.Error(),
			)
			return
		}
	}

	// populate Computed attributes
	plan.VMID = types.Int64Value(int64(vmr.VmId()))

	tflog.Trace(ctx, fmt.Sprintf("Setting state after creating VM to: %+v", plan))
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *vmResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vmResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !state.VMID.IsUnknown() {
		tflog.Trace(ctx, fmt.Sprintf("Reading state for VM %d", state.VMID.ValueInt64()))

		vms, err := pveapi.ListGuests(r.client)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading VM State",
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
			tflog.Trace(ctx, fmt.Sprintf("Can't read state of VM %d, it doesn't exist", state.VMID.ValueInt64()))
			resp.State.RemoveResource(ctx)
			return
		}

		err = UpdateResourceModelFromAPI(ctx, int(state.VMID.ValueInt64()), r.client, &state, VMStateEverything)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading VM State",
				fmt.Sprintf("Could not read state of VM %d, unsepcted error:"+err.Error(), state.VMID.ValueInt64()),
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

func (r *vmResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan vmResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Updating VM with plan: %+v", plan))

	config := &pveapi.ConfigQemu{}
	apiConfigFromResourceModel(&plan, config)

	id, err := getIDToUse(&plan, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Determining VM ID",
			"Unexpected error when getting next free VM ID from the API. If you can't solve this error please report it to the provider developers.\n\n"+err.Error())
		return
	}
	vmr := pveapi.NewVmRef(id)
	vmr.SetNode(plan.Node.ValueString())

	_, err = config.Update(false, vmr, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating VM",
			"Could not update VM, unexpected error: "+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("VM %d updated", id))

	reboot, err := pveapi.GuestHasPendingChanges(vmr, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating VM",
			"Unable to determine if VM needs reboot after updating it, unexpected error: "+err.Error(),
		)
		return
	}
	if reboot {
		// RebootVm (ie POST ../status/reboot) hangs and never completes, probably because we're testing on VMs with nothing installed
		tflog.Trace(ctx, fmt.Sprintf("Rebooting VM %d...", id))

		_, err = r.client.StopVm(vmr)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating VM",
				"Could not stop VM as part of reboot after updating it, unexpected error: "+err.Error(),
			)
			return
		}

		_, err = r.client.StartVm(vmr)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating VM",
				"Could not start VM as part of reboot after updating it, unexpected error: "+err.Error(),
			)
			return
		}

		tflog.Trace(ctx, fmt.Sprintf("Rebooted VM %d.", id))
	}

	var state vmResourceModel
	err = UpdateResourceModelFromAPI(ctx, id, r.client, &state, VMStateEverything)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating VM",
			"Could not read back updated VM status, unexpected error: "+err.Error(),
		)
		return
	}

	if plan.Status.ValueString() != state.Status.ValueString() {
		switch plan.Status.ValueString() {
		case stateRunning:
			tflog.Trace(ctx, "Starting VM since status in plan set to "+plan.Status.ValueString())
			_, err := r.client.StartVm(vmr)
			if err != nil {
				resp.Diagnostics.AddError(
					"Error Updating VM",
					"Could not start VM, unexpected error: "+err.Error(),
				)
				return
			}
		case stateStopped:
			tflog.Trace(ctx, "Starting VM since status in plan set to "+plan.Status.ValueString())
			_, err := r.client.StopVm(vmr)
			if err != nil {
				resp.Diagnostics.AddError(
					"Error Updating VM",
					"Could not stop VM, unexpected error: "+err.Error(),
				)
				return
			}
		}
	}

	err = UpdateResourceModelFromAPI(ctx, id, r.client, &state, VMStateStatus)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating VM",
			"Could not read back updated VM status, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Setting state after updating VM to: %+v", state))
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *vmResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vmResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	const deleteErrorSummary string = "Error Deleting VM"
	tflog.Trace(ctx, fmt.Sprintf("Deleting VM %d", state.VMID.ValueInt64()))

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
		tflog.Trace(ctx, fmt.Sprintf("Can't delete VM %d, doesn't exist", state.VMID.ValueInt64()))
		return
	}

	vmr := pveapi.NewVmRef(int(state.VMID.ValueInt64()))
	vmr.SetNode(state.Node.ValueString())

	// Does this fail if VM is stopped?
	_, err = r.client.StopVm(vmr)
	if err != nil {
		resp.Diagnostics.AddError(
			deleteErrorSummary,
			"Could not stop VM before deleting, unexpected error: "+err.Error(),
		)
		return
	}

	_, err = r.client.DeleteVm(vmr)
	if err != nil {
		resp.Diagnostics.AddError(
			deleteErrorSummary,
			"Could not delete VM, unexpected error: "+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("VM %d deleted", vmr.VmId()))
}

func (*vmResource) ImportState(_ context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.AddError(
		"ImportState Not Yet Supported",
		"Importing existing VM state is not currently supported, PRs welcome. :-)",
	)
}

func UpdateResourceModelFromAPI(ctx context.Context, vmid int, client *pveapi.Client, model *vmResourceModel, sm VMStateMask) error {
	vmr := pveapi.NewVmRef(vmid)

	tflog.Trace(ctx, "Updating vmResourceModel from PVE API.", map[string]any{"vmid": vmid, "statemask": sm})

	var config *pveapi.ConfigQemu
	var err error
	if sm&VMStateConfig != 0 {
		config, err = pveapi.NewConfigQemuFromApi(vmr, client)
		if err != nil {
			return err
		}
		tflog.Trace(ctx, fmt.Sprintf(".. updated config: %+v", config))
	}

	var status string
	if sm&VMStateStatus != 0 {
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

	if sm&VMStateConfig != 0 {
		model.Node = types.StringValue(config.Node)
		model.VMID = types.Int64Value(int64(config.VmID))

		if config.Name == "" {
			model.Name = types.StringNull()
		} else {
			model.Name = types.StringValue(config.Name)
		}

		if config.Description == "" {
			model.Description = types.StringNull()
		} else {
			model.Description = types.StringValue(config.Description)
		}

		model.Memory = types.Int64Value(int64(config.Memory))
	}
	if sm&VMStateStatus != 0 {
		model.Status = types.StringValue(status)
	}

	tflog.Trace(ctx, fmt.Sprintf("Updated vmResourceModel from PVE API, model is now %+v", model), map[string]any{"vmid": vmid, "statemask": sm})

	return nil
}

func apiConfigFromResourceModel(model *vmResourceModel, config *pveapi.ConfigQemu) {
	// Node set via VmRef
	// VMID set via VmRef
	config.Name = model.Name.ValueString()
	config.Description = model.Description.ValueString()

	config.Memory = int(model.Memory.ValueInt64())
}

func getIDToUse(model *vmResourceModel, client *pveapi.Client) (id int, err error) {
	const initialVMID = 100

	if !model.VMID.IsUnknown() {
		id = int(model.VMID.ValueInt64())
	} else {
		id, err = client.GetNextID(initialVMID)
		if err != nil {
			return 0, err
		}
	}

	return id, nil
}
