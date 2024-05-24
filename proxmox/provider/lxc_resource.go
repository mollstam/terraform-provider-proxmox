package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
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

	Hostname      types.String `tfsdk:"hostname"`
	Password      types.String `tfsdk:"password"`
	SSHPublicKeys types.String `tfsdk:"ssh_public_keys"`

	RootFs types.Object `tfsdk:"rootfs"`

	Net types.Object `tfsdk:"net"`
}

type rootfsModel struct {
	Volume  types.String `tfsdk:"volume"`
	Storage types.String `tfsdk:"storage"`
	Size    types.String `tfsdk:"size"`
}

func (rootfsModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"volume":  types.StringType,
		"storage": types.StringType,
		"size":    types.StringType,
	}
}

func (m *rootfsModel) readFromAPIConfig(c *pveapi.QemuDevice) {
	if val, ok := (*c)["volume"]; ok && val != "" {
		m.Volume = types.StringValue(val.(string))
		parts := strings.Split(val.(string), ":")
		m.Storage = types.StringValue(parts[0])
		if len(parts) == 2 {
			size, err := strconv.ParseInt(parts[1], 10, 64)
			if err == nil { // if eg "local-lvm:3" read it as 3G size
				m.Size = types.StringValue(fmt.Sprintf("%dG", size))
			}
		}
	} else if val, ok := (*c)["storage"]; ok {
		m.Storage = types.StringValue(val.(string))
	}
	if val, ok := (*c)["size"]; ok {
		m.Size = types.StringValue(val.(string))
	}
}

func (m rootfsModel) writeToAPIConfig(c *pveapi.QemuDevice) {
	(*c)["size"] = m.Size.ValueString()
	if !m.Volume.IsUnknown() {
		(*c)["volume"] = m.Volume.ValueString()
		parts := strings.Split(m.Volume.ValueString(), ":")
		(*c)["storage"] = parts[0]
	} else {
		(*c)["storage"] = m.Storage.ValueString()
	}
}

type lxcNetModel struct {
	Name    types.String `tfsdk:"name"`
	Bridge  types.String `tfsdk:"bridge"`
	IP      types.String `tfsdk:"ip"`
	Gateway types.String `tfsdk:"gw"`
}

func (lxcNetModel) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":   types.StringType,
		"bridge": types.StringType,
		"ip":     types.StringType,
		"gw":     types.StringType,
	}
}

func (m *lxcNetModel) readFromAPIConfig(c *pveapi.QemuDevice) {
	if val, ok := (*c)["name"]; ok {
		m.Name = types.StringValue(val.(string))
	}
	if val, ok := (*c)["bridge"]; ok {
		m.Bridge = types.StringValue(val.(string))
	}
	if val, ok := (*c)["ip"]; ok {
		m.IP = types.StringValue(val.(string))
	}
	if val, ok := (*c)["gw"]; ok && val != "" {
		m.Gateway = types.StringValue(val.(string))
	}
}

func (m lxcNetModel) writeToAPIConfig(c *pveapi.QemuDevice) {
	(*c)["name"] = m.Name.ValueString()
	(*c)["bridge"] = m.Bridge.ValueString()
	(*c)["ip"] = m.IP.ValueString()
	if !m.Gateway.IsUnknown() {
		(*c)["gw"] = m.Gateway.ValueString()
	}
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"hostname": schema.StringAttribute{
				Description: "Set a host name for the container.",
				Computed:    true,
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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
			"ssh_public_keys": schema.StringAttribute{
				Description: "Setup public SSH keys (one key per line, OpenSSH format).",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"rootfs": schemaRootFs(),
			"net":    schemaLxcNet(),
		},
	}
}

func schemaRootFs() schema.Attribute {
	return schema.SingleNestedAttribute{
		Description: "Use volume as container root.",
		Computed:    true,
		Optional:    true,
		Attributes: map[string]schema.Attribute{
			"volume": schema.StringAttribute{
				Description: "Volume identifier.",
				Computed:    true,
			},
			"storage": schema.StringAttribute{
				Description: "The storage identifier.",
				Required:    true,
			},
			"size": schema.StringAttribute{
				Description: "Size in kilobyte (1024 bytes). Optional suffixes 'M' (megabyte, 1024K) and 'G' (gigabyte, 1024M)",
				Required:    true,
				Validators: []validator.String{
					DiskSizeValidator("size must be numbers only, possibly ending in M or G"),
				},
			},
		},
		PlanModifiers: []planmodifier.Object{
			objectplanmodifier.UseStateForUnknown(),
		},
	}
}

func schemaLxcNet() schema.Attribute {
	return schema.SingleNestedAttribute{
		Description: "Specifies the network interface for the container.",
		Optional:    true,
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Network interface name.",
				Required:    true,
			},
			"bridge": schema.StringAttribute{
				Description: "The interface to bridge this interface to.",
				Required:    true,
			},
			"ip": schema.StringAttribute{
				Description: "IPv4 CIDR or \"dhcp\".",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.Any(
						IPCidrValidator("ip must be an IPv4 address with netmask in CIDR notation"),
						stringvalidator.OneOf("dhcp"),
					),
				},
			},
			"gw": schema.StringAttribute{
				Description: "IPv4",
				Optional:    true,
				Validators: []validator.String{
					IPValidator("gw must be an IPv4 address"),
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

	var vmr *pveapi.VmRef

	for {
		id, err := getIDToUse(plan.VMID, r.client)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Determining VM ID",
				"Unexpected error when getting next free VM ID from the API. If you can't solve this error please report it to the provider developers.\n\n"+err.Error())
			return
		}
		tflog.Trace(ctx, fmt.Sprintf("Creating with VMID %d", id))
		vmr = pveapi.NewVmRef(id)
		vmr.SetNode(plan.Node.ValueString())
		vmr.SetVmType(vmTypeLxc)

		err = config.CreateLxc(vmr, r.client)
		if err != nil {
			re := regexp.MustCompile(`unable to create CT \d+ \- CT \d+ already exists`)
			if plan.VMID.IsUnknown() && re.MatchString(err.Error()) {
				// if we tried creating with an auto-assigned ID try again
				continue
			}

			resp.Diagnostics.AddError(
				"Error Creating LXC",
				"Could not create LXC, unexpected error: "+err.Error(),
			)
			return
		}
		tflog.Trace(ctx, "Created LXC")
		break
	}

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

	// ensure Computed attributes get set, configured attributes should remain stable
	err = UpdateLXCResourceModelFromAPI(ctx, vmr.VmId(), r.client, &plan, LXCStateConfig)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating LXC",
			fmt.Sprintf("Could not read back state of created LXC %d, unexpected error:"+err.Error(), plan.VMID.ValueInt64()),
		)
		return
	}

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
				fmt.Sprintf("Could not read state of LXC %d, unexpected error:"+err.Error(), state.VMID.ValueInt64()),
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

	var state lxcResourceModel
	diags = req.State.Get(ctx, &state)
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
	vmr.SetVmType(vmTypeLxc)

	if state.RootFs.IsNull() != plan.RootFs.IsNull() || !state.RootFs.Equal(plan.RootFs) {
		oldRootfs, err := rootfsAPIConfigFromStateValue(ctx, state.RootFs)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error constructing API struct from internal model",
				"This is a provider bug. Please report it to the developers.\n\n"+err.Error())
			return
		}

		newRootfs, err := rootfsAPIConfigFromStateValue(ctx, plan.RootFs)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error constructing API struct from internal model",
				"This is a provider bug. Please report it to the developers.\n\n"+err.Error())
			return
		}

		oldDisks := pveapi.QemuDevices{0: oldRootfs}
		newDisks := pveapi.QemuDevices{0: newRootfs}
		err = applyLxcDiskChanges(oldDisks, newDisks, vmr, r.client)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating LXC",
				"Could not update LXC disks, unexpected error: "+err.Error(),
			)
			return
		}
		config.RootFs = newRootfs
	}

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

	var newState lxcResourceModel

	// carry over values not part of PVE state
	newState.Ostemplate = state.Ostemplate
	newState.Password = state.Password
	newState.SSHPublicKeys = state.SSHPublicKeys

	err = UpdateLXCResourceModelFromAPI(ctx, id, r.client, &newState, LXCStateEverything)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating LXC",
			"Could not read back updated LXC status, unexpected error: "+err.Error(),
		)
		return
	}

	if plan.Status.ValueString() != newState.Status.ValueString() {
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

	err = UpdateLXCResourceModelFromAPI(ctx, id, r.client, &newState, LXCStateStatus)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating LXC",
			"Could not read back updated LXC status, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Setting state after updating LXC to: %+v", newState))
	diags = resp.State.Set(ctx, newState)
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
	vmr.SetVmType(vmTypeLxc)

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

		if len(config.RootFs) == 0 {
			dm := rootfsModel{}
			dmAttrs := dm.AttributeTypes()
			model.RootFs = types.ObjectNull(dmAttrs)
		} else {
			dm := rootfsModel{}
			dm.readFromAPIConfig(&config.RootFs)
			m, diags := types.ObjectValueFrom(ctx, dm.AttributeTypes(), dm)
			if diags.HasError() {
				return errors.New("Unexpected error when reading rootfs from config")
			}
			model.RootFs = m
		}

		if len(config.Networks) == 0 {
			dm := lxcNetModel{}
			dmAttrs := dm.AttributeTypes()
			model.Net = types.ObjectNull(dmAttrs)
		} else {
			dm := lxcNetModel{}
			net0 := config.Networks[0]
			dm.readFromAPIConfig(&net0)
			m, diags := types.ObjectValueFrom(ctx, dm.AttributeTypes(), dm)
			if diags.HasError() {
				return errors.New("Unexpected error when reading net from config")
			}
			model.Net = m
		}
	}

	if sm&LXCStateStatus != 0 {
		model.Status = types.StringValue(status)
	}

	tflog.Trace(ctx, fmt.Sprintf("Updated lxcResourceModel from PVE API, model is now %+v", model), map[string]any{"vmid": vmid})

	return nil
}

func apiConfigFromLXCResourceModel(ctx context.Context, model *lxcResourceModel, config *pveapi.ConfigLxc) error {
	// Node set via VmRef
	// VMID set via VmRef
	config.Ostemplate = model.Ostemplate.ValueString()

	if !model.Hostname.IsNull() && !model.Hostname.IsUnknown() {
		config.Hostname = model.Hostname.ValueString()
	}

	if !model.Password.IsNull() && !model.Password.IsUnknown() {
		config.Password = model.Password.ValueString()
	}

	if !model.SSHPublicKeys.IsNull() && !model.SSHPublicKeys.IsUnknown() {
		config.SSHPublicKeys = model.SSHPublicKeys.ValueString()
	}

	if !model.Unprivileged.IsNull() && !model.Unprivileged.IsUnknown() {
		config.Unprivileged = model.Unprivileged.ValueBool()
	}

	var err error
	if !model.RootFs.IsNull() && !model.RootFs.IsUnknown() {
		config.RootFs, err = rootfsAPIConfigFromStateValue(ctx, model.RootFs)
		if err != nil {
			return err
		}
	}

	if !model.Net.IsNull() && !model.Net.IsUnknown() {
		net0, err := lxcNetAPIConfigFromStateValue(ctx, model.Net)
		if err != nil {
			return err
		}
		config.Networks = pveapi.QemuDevices{0: net0}
	}

	return nil
}

func rootfsAPIConfigFromStateValue(ctx context.Context, o basetypes.ObjectValue) (pveapi.QemuDevice, error) {
	if o.IsNull() {
		return nil, nil
	}

	var dm rootfsModel
	diags := o.As(ctx, &dm, basetypes.ObjectAsOptions{})
	if diags.HasError() {
		return nil, errors.New("unable to create config object from virtio0 state value")
	}
	c := pveapi.QemuDevice{}
	dm.writeToAPIConfig(&c)
	return c, nil
}

func lxcNetAPIConfigFromStateValue(ctx context.Context, o basetypes.ObjectValue) (pveapi.QemuDevice, error) {
	if o.IsNull() {
		return nil, nil
	}

	var dm lxcNetModel
	diags := o.As(ctx, &dm, basetypes.ObjectAsOptions{})
	if diags.HasError() {
		return nil, errors.New("unable to create config object from net state value")
	}
	c := pveapi.QemuDevice{}
	dm.writeToAPIConfig(&c)
	return c, nil
}

func applyLxcDiskChanges(prevDisks, newDisks pveapi.QemuDevices, vmr *pveapi.VmRef, c *pveapi.Client) error {
	// 1. Delete slots that either a. Don't exist in the new set or b. Have a different volume in the new set
	deleteDisks := []pveapi.QemuDevice{}
	for key, prevDisk := range prevDisks {
		newDisk, ok := (newDisks)[key]
		// The Rootfs can't be deleted
		if ok && diskSlotName(newDisk) == "rootfs" {
			continue
		}
		if !ok || (newDisk["volume"] != "" && prevDisk["volume"] != newDisk["volume"]) || (prevDisk["slot"] != newDisk["slot"]) {
			deleteDisks = append(deleteDisks, prevDisk)
		}
	}
	if len(deleteDisks) > 0 {
		deleteDiskKeys := []string{}
		for _, disk := range deleteDisks {
			deleteDiskKeys = append(deleteDiskKeys, diskSlotName(disk))
		}
		params := map[string]any{}
		params["delete"] = strings.Join(deleteDiskKeys, ", ")
		if vmr.GetVmType() == vmTypeLxc {
			if _, err := c.SetLxcConfig(vmr, params); err != nil {
				return err
			}
		} else {
			if _, err := c.SetVmConfig(vmr, params); err != nil {
				return err
			}
		}
	}

	// Create New Disks and Re-reference Slot-Changed Disks
	newParams := map[string]any{}
	for key, newDisk := range newDisks {
		prevDisk, ok := prevDisks[key]
		diskName := diskSlotName(newDisk)

		if ok {
			for k, v := range prevDisk {
				_, ok := newDisk[k]
				if !ok {
					newDisk[k] = v
				}
			}
		}

		if !ok || newDisk["slot"] != prevDisk["slot"] {
			newParams[diskName] = pveapi.FormatDiskParam(newDisk)
		}
	}
	if len(newParams) > 0 {
		if vmr.GetVmType() == vmTypeLxc {
			if _, err := c.SetLxcConfig(vmr, newParams); err != nil {
				return err
			}
		} else {
			if _, err := c.SetVmConfig(vmr, newParams); err != nil {
				return err
			}
		}
	}

	// Move and Resize Existing Disks
	for key, prevDisk := range prevDisks {
		newDisk, ok := newDisks[key]
		diskName := diskSlotName(newDisk)
		if ok {
			// 2. Move disks with mismatching storage
			newStorage, ok := newDisk["storage"].(string)
			if ok && newStorage != prevDisk["storage"] {
				if vmr.GetVmType() == vmTypeLxc {
					_, err := c.MoveLxcDisk(vmr, diskSlotName(prevDisk), newStorage)
					if err != nil {
						return err
					}
				} else {
					_, err := c.MoveQemuDisk(vmr, diskSlotName(prevDisk), newStorage)
					if err != nil {
						return err
					}
				}
			}

			// 3. Resize disks with different sizes
			if err := processDiskResize(prevDisk, newDisk, diskName, vmr, c); err != nil {
				return err
			}
		}
	}

	// Update Volume info
	apiResult, err := c.GetVmConfig(vmr)
	if err != nil {
		return err
	}
	for _, newDisk := range newDisks {
		diskName := diskSlotName(newDisk)
		apiConfigStr, ok := apiResult[diskName].(string)
		if !ok {
			return fmt.Errorf("Unable to read disk config from API as string, got a %T", apiResult[diskName])
		}
		apiDevice := pveapi.ParsePMConf(apiConfigStr, "volume")
		newDisk["volume"] = apiDevice["volume"]
	}

	return nil
}

func diskSlotName(disk pveapi.QemuDevice) string {
	diskType, ok := disk["type"].(string)
	if !ok || diskType == "" {
		diskType = "mp"
	}
	diskSlot, ok := disk["slot"].(int)
	if !ok {
		return "rootfs"
	}
	return diskType + strconv.Itoa(diskSlot)
}

func processDiskResize(prevDisk pveapi.QemuDevice, newDisk pveapi.QemuDevice, diskName string, vmr *pveapi.VmRef, c *pveapi.Client) error {
	newSize, ok := newDisk["size"]
	if ok && newSize != prevDisk["size"] {
		_, err := c.ResizeQemuDiskRaw(vmr, diskName, newDisk["size"].(string))
		if err != nil {
			return err
		}
	}
	return nil
}
