package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/mollstam/terraform-provider-proxmox/proxmox/provider"
)

// Run "go generate" to format example terraform files and generate the docs for the registry/website

// If you do not have terraform installed, you can remove the formatting command, but its suggested to
// ensure the documentation is formatted properly.
//go:generate terraform fmt -recursive ./examples/

// Run the docs generation tool, check its repository for more information on how it works and how docs
// can be customized.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs

var (
	// these will be set by the goreleaser configuration
	// to appropriate values for the compiled binary.
	version string = "dev"

	// goreleaser can pass other information to the main package, such as the specific commit
	// https://goreleaser.com/cookbooks/using-main.version/
)

func main() {

	var debug bool
	var pluginPath string

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.StringVar(&pluginPath, "registry", "registry.terraform.io/mollstam/proxmox", "specify path, useful for local debugging")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: pluginPath,
		Debug:   debug,
	}

	err := providerserver.Serve(
		context.Background(),
		provider.New(version),
		opts,
	)

	if err != nil {
		log.Fatal(err.Error())
	}
}
