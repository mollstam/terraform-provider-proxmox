package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	pveapi "github.com/mollstam/proxmox-api-go/proxmox"
	"github.com/onsi/gomega"
)

// testVLANTrunks builds the state value a vlan_trunks attribute holds for the given VLAN IDs.
func testVLANTrunks(ids ...int64) basetypes.ListValue {
	if len(ids) == 0 {
		return types.ListNull(vlanTrunksElementType)
	}

	elements := make([]attr.Value, 0, len(ids))
	for _, id := range ids {
		elements = append(elements, types.Int64Value(id))
	}

	return types.ListValueMust(vlanTrunksElementType, elements)
}

func TestReadVLANFromAPIConfig(t *testing.T) {
	tests := []struct {
		name       string
		device     pveapi.QemuDevice
		vlanID     basetypes.Int64Value
		vlanTrunks basetypes.ListValue
	}{
		{
			name:       "no vlan config",
			device:     pveapi.QemuDevice{"bridge": "vmbr0"},
			vlanID:     types.Int64Null(),
			vlanTrunks: testVLANTrunks(),
		},
		{
			name:       "tag only",
			device:     pveapi.QemuDevice{"bridge": "vmbr0", "tag": 100},
			vlanID:     types.Int64Value(100),
			vlanTrunks: testVLANTrunks(),
		},
		{
			// the API client parses a lone trunk into an int rather than a string
			name:       "single trunk",
			device:     pveapi.QemuDevice{"bridge": "vmbr0", "trunks": 20},
			vlanID:     types.Int64Null(),
			vlanTrunks: testVLANTrunks(20),
		},
		{
			name:       "several trunks",
			device:     pveapi.QemuDevice{"bridge": "vmbr0", "trunks": "20;30;40"},
			vlanID:     types.Int64Null(),
			vlanTrunks: testVLANTrunks(20, 30, 40),
		},
		{
			name:       "tag and trunks",
			device:     pveapi.QemuDevice{"bridge": "vmbr0", "tag": 100, "trunks": "20;30"},
			vlanID:     types.Int64Value(100),
			vlanTrunks: testVLANTrunks(20, 30),
		},
		{
			name:       "trunks read back as float64",
			device:     pveapi.QemuDevice{"bridge": "vmbr0", "tag": float64(100), "trunks": float64(20)},
			vlanID:     types.Int64Value(100),
			vlanTrunks: testVLANTrunks(20),
		},
		{
			name:       "unparsable values are ignored",
			device:     pveapi.QemuDevice{"bridge": "vmbr0", "tag": "nope", "trunks": "nope"},
			vlanID:     types.Int64Null(),
			vlanTrunks: testVLANTrunks(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			vlanID, vlanTrunks := readVLANFromAPIConfig(&tt.device)

			g.Expect(vlanID).To(gomega.Equal(tt.vlanID))
			g.Expect(vlanTrunks).To(gomega.Equal(tt.vlanTrunks))
		})
	}
}

func TestWriteVLANToAPIConfig(t *testing.T) {
	tests := []struct {
		name       string
		vlanID     basetypes.Int64Value
		vlanTrunks basetypes.ListValue
		device     pveapi.QemuDevice
	}{
		{
			name:       "unset leaves keys out",
			vlanID:     types.Int64Null(),
			vlanTrunks: testVLANTrunks(),
			device:     pveapi.QemuDevice{},
		},
		{
			name:       "unknown leaves keys out",
			vlanID:     types.Int64Unknown(),
			vlanTrunks: types.ListUnknown(vlanTrunksElementType),
			device:     pveapi.QemuDevice{},
		},
		{
			name:       "tag only",
			vlanID:     types.Int64Value(100),
			vlanTrunks: testVLANTrunks(),
			device:     pveapi.QemuDevice{"tag": 100},
		},
		{
			name:       "trunks are joined by semicolon",
			vlanID:     types.Int64Null(),
			vlanTrunks: testVLANTrunks(20, 30, 40),
			device:     pveapi.QemuDevice{"trunks": "20;30;40"},
		},
		{
			name:       "trunk order is kept",
			vlanID:     types.Int64Null(),
			vlanTrunks: testVLANTrunks(40, 20, 30),
			device:     pveapi.QemuDevice{"trunks": "40;20;30"},
		},
		{
			name:       "tag and trunks",
			vlanID:     types.Int64Value(100),
			vlanTrunks: testVLANTrunks(20, 30),
			device:     pveapi.QemuDevice{"tag": 100, "trunks": "20;30"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			device := pveapi.QemuDevice{}
			writeVLANToAPIConfig(&device, tt.vlanID, tt.vlanTrunks)

			g.Expect(device).To(gomega.Equal(tt.device))
		})
	}
}

func TestVLANRoundTripsThroughAPIConfig(t *testing.T) {
	g := gomega.NewWithT(t)

	device := pveapi.QemuDevice{}
	writeVLANToAPIConfig(&device, types.Int64Value(100), testVLANTrunks(20, 30, 40))

	vlanID, vlanTrunks := readVLANFromAPIConfig(&device)

	g.Expect(vlanID).To(gomega.Equal(types.Int64Value(100)))
	g.Expect(vlanTrunks).To(gomega.Equal(testVLANTrunks(20, 30, 40)))
}
