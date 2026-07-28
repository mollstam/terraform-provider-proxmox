package provider

import (
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	pveapi "github.com/mollstam/proxmox-api-go/proxmox"
)

const (
	vlanIDMin int64 = 1
	vlanIDMax int64 = 4094

	// PVE joins the VLAN IDs of a trunk list with semicolons in the netX config string.
	vlanTrunksSeparator string = ";"
)

// vlanTrunksElementType is the element type of the vlan_trunks attribute.
var vlanTrunksElementType = types.Int64Type

func schemaVLANID() schema.Attribute {
	return schema.Int64Attribute{
		Description: "The VLAN tag to apply to packets on this interface.",
		Optional:    true,
		Validators: []validator.Int64{
			int64validator.Between(vlanIDMin, vlanIDMax),
		},
	}
}

func schemaVLANTrunks() schema.Attribute {
	return schema.ListAttribute{
		Description: "The VLAN IDs to pass through this interface.",
		Optional:    true,
		ElementType: vlanTrunksElementType,
		Validators: []validator.List{
			listvalidator.SizeAtLeast(1),
			listvalidator.UniqueValues(),
			listvalidator.ValueInt64sAre(int64validator.Between(vlanIDMin, vlanIDMax)),
		},
	}
}

// readVLANFromAPIConfig reads the tag and trunks keys of a network device config.
func readVLANFromAPIConfig(c *pveapi.QemuDevice) (types.Int64, types.List) {
	vlanID := types.Int64Null()
	if val, ok := (*c)["tag"]; ok {
		if id, ok := vlanIDFromAPIConfigValue(val); ok {
			vlanID = types.Int64Value(id)
		}
	}

	vlanTrunks := types.ListNull(vlanTrunksElementType)
	if val, ok := (*c)["trunks"]; ok {
		var ids []attr.Value
		// a lone trunk is parsed into an int by the API client, several stay a semicolon separated string
		if id, ok := vlanIDFromAPIConfigValue(val); ok {
			ids = append(ids, types.Int64Value(id))
		} else if s, ok := val.(string); ok {
			for _, part := range strings.Split(s, vlanTrunksSeparator) {
				id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
				if err != nil {
					continue
				}
				ids = append(ids, types.Int64Value(id))
			}
		}
		if len(ids) > 0 {
			vlanTrunks = types.ListValueMust(vlanTrunksElementType, ids)
		}
	}

	return vlanID, vlanTrunks
}

// writeVLANToAPIConfig sets the tag and trunks keys of a network device config, leaving
// them out when unset so that PVE drops any VLAN configuration the interface used to have.
func writeVLANToAPIConfig(c *pveapi.QemuDevice, vlanID types.Int64, vlanTrunks types.List) {
	if !vlanID.IsNull() && !vlanID.IsUnknown() {
		(*c)["tag"] = int(vlanID.ValueInt64())
	}

	if !vlanTrunks.IsNull() && !vlanTrunks.IsUnknown() {
		elements := vlanTrunks.Elements()
		ids := make([]string, 0, len(elements))
		for _, e := range elements {
			id, ok := e.(types.Int64)
			if !ok || id.IsNull() || id.IsUnknown() {
				continue
			}
			ids = append(ids, strconv.FormatInt(id.ValueInt64(), 10))
		}
		if len(ids) > 0 {
			(*c)["trunks"] = strings.Join(ids, vlanTrunksSeparator)
		}
	}
}

func vlanIDFromAPIConfigValue(v any) (int64, bool) {
	switch val := v.(type) {
	case int:
		return int64(val), true
	case int64:
		return val, true
	case float64:
		return int64(val), true
	}

	return 0, false
}
