package provider

import (
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	pveapi "github.com/mollstam/proxmox-api-go/proxmox"
)

const initialVMID = 100

// vmidAllocator hands out free VMIDs to resources being created in parallel. PVE only
// considers an ID taken once the create request has written the guest config, so
// resources asking /cluster/nextid at the same time would otherwise all get the same ID.
// IDs handed out are remembered for the lifetime of the provider process.
type vmidAllocator struct {
	client *pveapi.Client

	mu       sync.Mutex
	reserved map[int]struct{}
}

func newVMIDAllocator(client *pveapi.Client) *vmidAllocator {
	return &vmidAllocator{
		client:   client,
		reserved: map[int]struct{}{},
	}
}

// Next returns a VMID that PVE reports as free and that has not already been handed out.
func (a *vmidAllocator) Next() (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	candidate := initialVMID
	for {
		id, err := a.client.GetNextID(candidate)
		if err != nil {
			return 0, err
		}
		if _, taken := a.reserved[id]; !taken {
			a.reserved[id] = struct{}{}
			return id, nil
		}
		candidate = id + 1
	}
}

// IDFor returns the configured VMID, or allocates a free one if it is left to be computed.
func (a *vmidAllocator) IDFor(v basetypes.Int64Value) (int, error) {
	if !v.IsUnknown() {
		return int(v.ValueInt64()), nil
	}
	return a.Next()
}

// Release gives up an ID returned by Next that ended up not being used.
func (a *vmidAllocator) Release(id int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.reserved, id)
}
