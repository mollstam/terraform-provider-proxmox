package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	pveapi "github.com/mollstam/proxmox-api-go/proxmox"
)

// newFakeNextIDServer serves /cluster/nextid like PVE does: an ID is only taken once it
// is in existing, which models the window between asking for an ID and the create
// request writing the guest config.
func newFakeNextIDServer(t *testing.T, existing map[int]bool) *pveapi.Client {
	t.Helper()

	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster/nextid" {
			http.NotFound(w, r)
			return
		}
		id, err := strconv.Atoi(r.URL.Query().Get("vmid"))
		if err != nil {
			http.Error(w, "bad vmid", http.StatusBadRequest)
			return
		}

		// give concurrent callers a chance to overlap, like a real round trip would
		time.Sleep(5 * time.Millisecond)

		mu.Lock()
		taken := existing[id]
		mu.Unlock()
		if taken {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"data":null,"errors":{"vmid":"VM %d already exists"}}`, id)
			return
		}
		fmt.Fprintf(w, `{"data":"%d"}`, id)
	}))
	t.Cleanup(srv.Close)

	client, err := pveapi.NewClient(srv.URL, nil, "", nil, "", 10)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	return client
}

func TestVMIDAllocatorConcurrentNextIsUnique(t *testing.T) {
	a := newVMIDAllocator(newFakeNextIDServer(t, map[int]bool{100: true, 101: true}))

	const n = 10
	ids := make([]int, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ids[i], errs[i] = a.Next()
		}(i)
	}
	wg.Wait()

	seen := map[int]bool{}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("Next returned error: %v", errs[i])
		}
		if ids[i] < 102 {
			t.Errorf("got ID %d which already exists in PVE", ids[i])
		}
		if seen[ids[i]] {
			t.Errorf("ID %d handed out more than once, got %v", ids[i], ids)
		}
		seen[ids[i]] = true
	}
}

func TestVMIDAllocatorReleaseMakesIDAvailableAgain(t *testing.T) {
	a := newVMIDAllocator(newFakeNextIDServer(t, map[int]bool{}))

	first, err := a.Next()
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.Next()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("expected distinct IDs, got %d twice", first)
	}

	a.Release(first)
	again, err := a.Next()
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Errorf("expected released ID %d to be handed out again, got %d", first, again)
	}
}
