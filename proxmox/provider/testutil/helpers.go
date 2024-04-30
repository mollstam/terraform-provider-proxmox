package testutil

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/terraform-plugin-log/tfsdklog"
	pveapi "github.com/mollstam/proxmox-api-go/proxmox"
	"github.com/onsi/gomega"
)

var TestClient *pveapi.Client

const (
	apiURL         string = "https://192.168.56.102:8006/api2/json"
	apiTokenID     string = "root@pam!tf"
	apiTokenSecret string = "897d5216-64c1-4da8-b6dc-33eed34a34a0"
	tlsInsecure    bool   = true
	httpHeaders    string = ""
	timeout        int    = 10
	proxy          string = ""
	debug          bool   = false
)

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
	tlsconf := &tls.Config{InsecureSkipVerify: true}
	if !tlsInsecure {
		tlsconf = nil
	}

	var err error
	if apiTokenSecret == "" {
		err = errors.New("API token secret not provided, must exist")
	}

	if !strings.Contains(apiTokenID, "!") {
		err = errors.New("your API Token ID should contain a !, check your API credentials")
	}
	if err != nil {
		return nil, err
	}

	client, _ := pveapi.NewClient(apiURL, nil, httpHeaders, tlsconf, proxy, timeout)
	*pveapi.Debug = debug

	client.SetAPIToken(apiTokenID, apiTokenSecret)

	return client, nil
}

func GetTestLoggingContext() context.Context {
	return tfsdklog.NewRootProviderLogger(context.Background(), tfsdklog.WithLogName("proxmox"), tfsdklog.WithLevel(hclog.Trace), tfsdklog.WithoutLocation())
}
