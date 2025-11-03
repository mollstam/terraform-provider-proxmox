package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

const (
	providerConfig = `
provider "proxmox" {
	api_url = "https://172.26.56.125:8006/api2/json"
	tls_insecure = true

	api_token_id = "root@pam!tf"
	api_token_secret = "4bac8712-4936-4739-a529-be2d5f1ac2de"
	
	proxy_server = "https://127.0.0.1:8006"

	debug = true
}
`
)

var (
	testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
		"proxmox": providerserver.NewProtocol6WithError(New("test")()),
	}
)
