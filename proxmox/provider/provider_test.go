package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

const (
	providerConfig = `
provider "proxmox" {
	api_url = "https://127.0.0.1:8806/api2/json"
	tls_insecure = true

	api_token_id = "root@pam!tf"
	api_token_secret = "897d5216-64c1-4da8-b6dc-33eed34a34a0"

	debug = false
	proxy_server = "http://127.0.0.1:8080"
}
`
)

var (
	testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
		"proxmox": providerserver.NewProtocol6WithError(New("test")()),
	}
)
