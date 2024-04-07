package testutil

import (
	"crypto/tls"
	"fmt"
	"strings"

	pveapi "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/onsi/gomega"
)

var TestClient *pveapi.Client

func init() {
	client, err := newProxmoxTestClient()
	if err != nil {
		panic("failed to create test client: " + err.Error())
	}
	TestClient = client

	// We are using the gomega package for its matchers only, but it requires us to register a handler anyway.
	gomega.RegisterFailHandler(func(_ string, _ ...int) {
		panic("gomega fail handler should not be used")
	})

}

func newProxmoxTestClient() (*pveapi.Client, error) {
	api_url := "https://192.168.56.102:8006/api2/json"
	api_token_id := "root@pam!tf"
	api_token_secret := "897d5216-64c1-4da8-b6dc-33eed34a34a0"
	tls_insecure := true
	http_headers := ""
	timeout := 10
	debug := true

	tlsconf := &tls.Config{InsecureSkipVerify: true}
	if !tls_insecure {
		tlsconf = nil
	}

	var err error
	if api_token_secret == "" {
		err = fmt.Errorf("API token secret not provided, must exist")
	}

	if !strings.Contains(api_token_id, "!") {
		err = fmt.Errorf("your API Token ID should contain a !, check your API credentials")
	}
	if err != nil {
		return nil, err
	}

	client, _ := pveapi.NewClient(api_url, nil, http_headers, tlsconf, "", timeout)
	*pveapi.Debug = debug

	client.SetAPIToken(api_token_id, api_token_secret)

	return client, nil
}
