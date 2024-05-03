package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
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

	mediaDisk  string = "disk"
	mediaCdrom string = "cdrom"

	formatRaw   string = "raw"
	formatCow   string = "cow"
	formatQcow  string = "qcow"
	formatQed   string = "qed"
	formatQcow2 string = "qcow2"
	formatVmdk  string = "vmdk"
	formatCloop string = "cloop"
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
	Agent  types.Bool   `tfsdk:"agent"`

	Clone types.Int64 `tfsdk:"clone"`

	Sockets types.Int64 `tfsdk:"sockets"`
	Cores   types.Int64 `tfsdk:"cores"`
	Memory  types.Int64 `tfsdk:"memory"`

	Virtio0  types.Object `tfsdk:"virtio0"`
	Virtio1  types.Object `tfsdk:"virtio1"`
	Virtio2  types.Object `tfsdk:"virtio2"`
	Virtio3  types.Object `tfsdk:"virtio3"`
	Virtio4  types.Object `tfsdk:"virtio4"`
	Virtio5  types.Object `tfsdk:"virtio5"`
	Virtio6  types.Object `tfsdk:"virtio6"`
	Virtio7  types.Object `tfsdk:"virtio7"`
	Virtio8  types.Object `tfsdk:"virtio8"`
	Virtio9  types.Object `tfsdk:"virtio9"`
	Virtio10 types.Object `tfsdk:"virtio10"`
	Virtio11 types.Object `tfsdk:"virtio11"`
	Virtio12 types.Object `tfsdk:"virtio12"`
	Virtio13 types.Object `tfsdk:"virtio13"`
	Virtio14 types.Object `tfsdk:"virtio14"`
	Virtio15 types.Object `tfsdk:"virtio15"`
}

type virtioModel struct {
	Media types.String `tfsdk:"media"`

	Format  types.String `tfsdk:"format"`
	Size    types.Int64  `tfsdk:"size"`
	Storage types.String `tfsdk:"storage"`
}

func (virtioModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"media":   types.StringType,
		"format":  types.StringType,
		"size":    types.Int64Type,
		"storage": types.StringType,
	}
}

func (m *virtioModel) readFromAPIConfig(c *pveapi.QemuVirtIOStorage) {
	m.Media = types.StringValue(mediaDisk)
	m.Storage = types.StringValue(c.Disk.Storage)
	m.Size = types.Int64Value(int64(c.Disk.SizeInKibibytes) / (1024 * 1024))
	m.Format = types.StringValue(string(c.Disk.Format))
}

func (m virtioModel) writeToAPIConfig(c *pveapi.QemuVirtIOStorage) {
	c.Disk = &pveapi.QemuVirtIODisk{
		Format:          pveapi.QemuDiskFormat(m.Format.ValueString()),
		Storage:         m.Storage.ValueString(),
		SizeInKibibytes: pveapi.QemuDiskSize(m.Size.ValueInt64() * 1024 * 1024),
	}
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
			"agent": schema.BoolAttribute{
				Description: "Enable/disable communication with the QEMU Guest Agent and its properties.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"sockets": schema.Int64Attribute{
				Description: "The number of CPU sockets.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(1),
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"cores": schema.Int64Attribute{
				Description: "The number of cores per socket.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(1),
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
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
			"virtio0":  schemaVirtio(),
			"virtio1":  schemaVirtio(),
			"virtio2":  schemaVirtio(),
			"virtio3":  schemaVirtio(),
			"virtio4":  schemaVirtio(),
			"virtio5":  schemaVirtio(),
			"virtio6":  schemaVirtio(),
			"virtio7":  schemaVirtio(),
			"virtio8":  schemaVirtio(),
			"virtio9":  schemaVirtio(),
			"virtio10": schemaVirtio(),
			"virtio11": schemaVirtio(),
			"virtio12": schemaVirtio(),
			"virtio13": schemaVirtio(),
			"virtio14": schemaVirtio(),
			"virtio15": schemaVirtio(),
		},
	}
}

func schemaVirtio() schema.Attribute {
	return schema.SingleNestedAttribute{
		Description: "Use volume as VIRTIO hard disk.",
		Optional:    true,
		Attributes: map[string]schema.Attribute{
			"media": schema.StringAttribute{
				Description: "The type of media for this volume (disk or cdrom).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf([]string{mediaDisk, mediaCdrom}...),
				},
			},
			"format": schema.StringAttribute{
				Description: "Format identifier (raw, cow, qcow, qed, qcow2, vmdk, cloop).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(formatRaw),
				Validators: []validator.String{
					stringvalidator.OneOf([]string{formatRaw, formatCow, formatQcow, formatQed, formatQcow2, formatVmdk, formatCloop}...),
				},
			},
			"size": schema.Int64Attribute{
				Description: "Volume size in GB.",
				Required:    true,
			},
			"storage": schema.StringAttribute{
				Description: "The storage identifier.",
				Required:    true,
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
	err := apiConfigFromResourceModel(ctx, &plan, config)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error constructing API struct from internal model",
			"This is a provider bug. Please report it to the developers.\n\n"+err.Error())
		return
	}
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
	err := apiConfigFromResourceModel(ctx, &plan, config)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error constructing API struct from internal model",
			"This is a provider bug. Please report it to the developers.\n\n"+err.Error())
		return
	}

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

		model.Agent = types.BoolValue(config.Agent > 0)
		model.Sockets = types.Int64Value(int64(config.QemuSockets))
		model.Cores = types.Int64Value(int64(config.QemuCores))
		model.Memory = types.Int64Value(int64(config.Memory))

		if config.Disks == nil || config.Disks.VirtIO == nil {
			dm := virtioModel{}
			dmAttrs := dm.AttributeTypes()
			model.Virtio0 = types.ObjectNull(dmAttrs)
			model.Virtio1 = types.ObjectNull(dmAttrs)
			model.Virtio2 = types.ObjectNull(dmAttrs)
			model.Virtio3 = types.ObjectNull(dmAttrs)
			model.Virtio4 = types.ObjectNull(dmAttrs)
			model.Virtio5 = types.ObjectNull(dmAttrs)
			model.Virtio6 = types.ObjectNull(dmAttrs)
			model.Virtio7 = types.ObjectNull(dmAttrs)
			model.Virtio8 = types.ObjectNull(dmAttrs)
			model.Virtio9 = types.ObjectNull(dmAttrs)
			model.Virtio10 = types.ObjectNull(dmAttrs)
			model.Virtio11 = types.ObjectNull(dmAttrs)
			model.Virtio12 = types.ObjectNull(dmAttrs)
			model.Virtio13 = types.ObjectNull(dmAttrs)
			model.Virtio14 = types.ObjectNull(dmAttrs)
			model.Virtio15 = types.ObjectNull(dmAttrs)
		} else {
			model.Virtio0, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_0)
			if err != nil {
				return err
			}

			model.Virtio1, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_1)
			if err != nil {
				return err
			}

			model.Virtio2, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_2)
			if err != nil {
				return err
			}

			model.Virtio3, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_3)
			if err != nil {
				return err
			}

			model.Virtio4, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_4)
			if err != nil {
				return err
			}

			model.Virtio5, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_5)
			if err != nil {
				return err
			}

			model.Virtio6, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_6)
			if err != nil {
				return err
			}

			model.Virtio7, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_7)
			if err != nil {
				return err
			}

			model.Virtio8, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_8)
			if err != nil {
				return err
			}

			model.Virtio9, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_9)
			if err != nil {
				return err
			}

			model.Virtio10, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_10)
			if err != nil {
				return err
			}

			model.Virtio11, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_11)
			if err != nil {
				return err
			}

			model.Virtio12, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_12)
			if err != nil {
				return err
			}

			model.Virtio13, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_13)
			if err != nil {
				return err
			}

			model.Virtio14, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_14)
			if err != nil {
				return err
			}

			model.Virtio15, err = virtioStateValueFromAPIConfig(ctx, config.Disks.VirtIO.Disk_15)
			if err != nil {
				return err
			}
		}
	}
	if sm&VMStateStatus != 0 {
		model.Status = types.StringValue(status)
	}

	tflog.Trace(ctx, fmt.Sprintf("Updated vmResourceModel from PVE API, model is now %+v", model), map[string]any{"vmid": vmid, "statemask": sm})

	return nil
}

func virtioStateValueFromAPIConfig(ctx context.Context, c *pveapi.QemuVirtIOStorage) (types.Object, error) {
	dm := virtioModel{} // create instance to gain access to AttributeTypes() below for nil branch...
	if c == nil {
		return types.ObjectNull(dm.AttributeTypes()), nil
	}

	dm.readFromAPIConfig(c)
	m, diags := types.ObjectValueFrom(ctx, dm.AttributeTypes(), dm)
	if diags.HasError() {
		return types.Object{}, errors.New("Unexpected error when reading virtio0 from config")
	}

	return m, nil
}

func apiConfigFromResourceModel(ctx context.Context, model *vmResourceModel, config *pveapi.ConfigQemu) error {
	// Node set via VmRef
	// VMID set via VmRef
	config.Name = model.Name.ValueString()
	config.Description = model.Description.ValueString()

	config.Agent = 0
	if model.Agent.ValueBool() {
		config.Agent = 1
	}

	config.QemuSockets = int(model.Sockets.ValueInt64())
	config.QemuCores = int(model.Cores.ValueInt64())
	config.Memory = int(model.Memory.ValueInt64())

	// even if we have no disks in state we need empty structs for API client to consider it and e.g. emit delete actions
	config.Disks = &pveapi.QemuStorages{
		VirtIO: &pveapi.QemuVirtIODisks{},
	}
	if !(model.Virtio0.IsNull() && model.Virtio1.IsNull() && model.Virtio2.IsNull() && model.Virtio3.IsNull() && model.Virtio4.IsNull() && model.Virtio5.IsNull() && model.Virtio6.IsNull() && model.Virtio7.IsNull() && model.Virtio8.IsNull() && model.Virtio9.IsNull() && model.Virtio10.IsNull() && model.Virtio11.IsNull() && model.Virtio12.IsNull() && model.Virtio13.IsNull() && model.Virtio14.IsNull() && model.Virtio15.IsNull()) {
		var err error

		config.Disks.VirtIO.Disk_0, err = virtioAPIConfigFromStateValue(ctx, model.Virtio0)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_1, err = virtioAPIConfigFromStateValue(ctx, model.Virtio1)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_2, err = virtioAPIConfigFromStateValue(ctx, model.Virtio2)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_3, err = virtioAPIConfigFromStateValue(ctx, model.Virtio3)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_4, err = virtioAPIConfigFromStateValue(ctx, model.Virtio4)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_5, err = virtioAPIConfigFromStateValue(ctx, model.Virtio5)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_6, err = virtioAPIConfigFromStateValue(ctx, model.Virtio6)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_7, err = virtioAPIConfigFromStateValue(ctx, model.Virtio7)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_8, err = virtioAPIConfigFromStateValue(ctx, model.Virtio8)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_9, err = virtioAPIConfigFromStateValue(ctx, model.Virtio9)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_10, err = virtioAPIConfigFromStateValue(ctx, model.Virtio10)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_11, err = virtioAPIConfigFromStateValue(ctx, model.Virtio11)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_12, err = virtioAPIConfigFromStateValue(ctx, model.Virtio12)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_13, err = virtioAPIConfigFromStateValue(ctx, model.Virtio13)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_14, err = virtioAPIConfigFromStateValue(ctx, model.Virtio14)
		if err != nil {
			return err
		}
		config.Disks.VirtIO.Disk_15, err = virtioAPIConfigFromStateValue(ctx, model.Virtio15)
		if err != nil {
			return err
		}
	}

	return nil
}

func virtioAPIConfigFromStateValue(ctx context.Context, o basetypes.ObjectValue) (*pveapi.QemuVirtIOStorage, error) {
	if o.IsNull() {
		return nil, nil
	}

	var dm virtioModel
	diags := o.As(ctx, &dm, basetypes.ObjectAsOptions{})
	if diags.HasError() {
		return nil, errors.New("unable to create config object from virtio0 state value")
	}
	c := &pveapi.QemuVirtIOStorage{}
	dm.writeToAPIConfig(c)
	return c, nil
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
