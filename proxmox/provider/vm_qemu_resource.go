package provider

import (
	"context"
	"fmt"

	pveapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &vmQemuResource{}
	_ resource.ResourceWithConfigure   = &vmQemuResource{}
	_ resource.ResourceWithImportState = &vmQemuResource{}
)

func NewVmQemuResource() resource.Resource {
	return &vmQemuResource{}
}

type vmQemuResource struct {
	client *pveapi.Client
}

type vmQemuResourceModel struct {
	Node types.String `tfsdk:"node"`
	VMID types.Int64  `tfsdk:"vmid"`
	Name types.String `tfsdk:"name"`
}

func (r *vmQemuResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_qemu"
}

func (r *vmQemuResource) Schema(_ context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This resource manages a Proxmox VM Qemu container.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Description: "The cluster node name",
				Required:    true,
			},
			"vmid": schema.Int64Attribute{
				Description: "The (unique) ID of the VM.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Set a name for the VM. Only used on the configuration web interface.",
				Optional:    true,
			},
		},
	}
}

func (r *vmQemuResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *vmQemuResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vmQemuResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := &pveapi.ConfigQemu{}
	ConfigFromResourceModel(&plan, config)
	tflog.Trace(ctx, fmt.Sprintf("Creating VM from model: %v+", plan))

	id, err := GetIdToUse(&plan, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Determining VM ID",
			"Unexpected error when getting next free VM ID from the API. If you can't solve this error please report it to the provider developers.\n\n"+err.Error())
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("Creating with VMID %d", id))
	ref := pveapi.NewVmRef(id)
	ref.SetNode(plan.Node.ValueString())

	err = config.Create(ref, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating VM",
			"Could not create VM, unexpected error: "+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, "Created VM")

	// populate Computed attributes
	plan.VMID = types.Int64Value(int64(ref.VmId()))

	tflog.Trace(ctx, fmt.Sprintf("Setting state after creating VM to: %v+", plan))
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *vmQemuResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vmQemuResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TODO things to test
	// - What happens when Refresh is run and the VM can no longer be found? Will vmid revert back to Unknown if not configured?
	// - If created with null name, will it read back as null or ""?
	// - A VM has been created with a configured ID but the ID is then unconfigured and computed. Keep ID or recreated with next free?

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

		ref := pveapi.NewVmRef(int(state.VMID.ValueInt64()))
		config, err := pveapi.NewConfigQemuFromApi(ref, r.client)
		if err != nil {

			resp.Diagnostics.AddError(
				"Error Reading VM",
				"Could not read VM, unexpected error: "+err.Error(),
			)
			return
		}

		ResourceModelFromConfig(config, &state)
		tflog.Trace(ctx, fmt.Sprintf("Read state %v+", state))
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *vmQemuResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan vmQemuResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Updating VM with plan: %v+", plan))

	config := &pveapi.ConfigQemu{}
	ConfigFromResourceModel(&plan, config)

	id, err := GetIdToUse(&plan, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Determining VM ID",
			"Unexpected error when getting next free VM ID from the API. If you can't solve this error please report it to the provider developers.\n\n"+err.Error())
		return
	}
	ref := pveapi.NewVmRef(id)
	ref.SetNode(plan.Node.ValueString())

	// we probably want to pass false here in the future and reboot ourselves when update is completed (original provider does this, mentioning cloud-init)
	_, err = config.Update(true, ref, r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating VM",
			"Could not update VM, unexpected error: "+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("VM %d updated", id))

	config, err = pveapi.NewConfigQemuFromApi(ref, r.client)
	if err != nil {

		resp.Diagnostics.AddError(
			"Error Updating VM",
			"Could not read back updated VM, unexpected error: "+err.Error(),
		)
		return
	}

	ResourceModelFromConfig(config, &plan)

	tflog.Trace(ctx, fmt.Sprintf("Setting state after updating VM to: %v+", plan))
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *vmQemuResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vmQemuResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Deleting VM %d", state.VMID.ValueInt64()))

	vms, err := pveapi.ListGuests(r.client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting VM",
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

	ref := pveapi.NewVmRef(int(state.VMID.ValueInt64()))
	ref.SetNode(state.Node.ValueString())

	// TODO this probably fails if the VM is running, make a test for it!
	status, err := r.client.DeleteVm(ref)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting VM",
			fmt.Sprintf("Could not delete VM, unexpected error (task status %s):", status)+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, fmt.Sprintf("VM %d deleted", ref.VmId()))
}

func (r *vmQemuResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.AddError(
		"ImportState Not Yet Supported",
		"Importing existing VM state is not currently supported, PRs welcome. :-)",
	)
}

func ConfigFromResourceModel(model *vmQemuResourceModel, config *pveapi.ConfigQemu) {
	// Node set via VmRef
	config.Name = model.Name.ValueString()
}

func ResourceModelFromConfig(config *pveapi.ConfigQemu, model *vmQemuResourceModel) {
	model.Node = types.StringValue(config.Node)
	model.VMID = types.Int64Value(int64(config.VmID))
	model.Name = types.StringValue(config.Name)
}

func GetIdToUse(model *vmQemuResourceModel, client *pveapi.Client) (id int, err error) {
	if !model.VMID.IsUnknown() {
		id = int(model.VMID.ValueInt64())
	} else {
		id, err = client.GetNextID(100)
		if err != nil {
			return 0, err
		}
	}

	return id, nil
}
