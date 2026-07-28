# GNU Make on Windows runs recipes through cmd.exe, which shares no syntax with sh.
# Everything below is written to work under both, using the command variables in
# this first section to paper over the differences. Keep new recipes portable.
ifeq ($(OS),Windows_NT)
# pin the shell so it does not matter whether make was launched from cmd, PowerShell or Git Bash
SHELL := $(if $(COMSPEC),$(COMSPEC),cmd.exe)
NULL_DEV := nul
# cmd builtins only understand backslash separated paths
FIXPATH  = $(subst /,\,$1)
MKDIR_P  = if not exist "$(call FIXPATH,$1)" mkdir "$(call FIXPATH,$1)"
RM_RF    = if exist "$(call FIXPATH,$1)" rmdir /s /q "$(call FIXPATH,$1)"
COPY     = copy /y "$(call FIXPATH,$1)" "$(call FIXPATH,$2)" >$(NULL_DEV)
SETENV   = set $1&&
# make execs simple commands directly, so cmd builtins need an explicit cmd /c
INC      = $(shell cmd /c set /a $1+1)
# findstr exits 0 when it matched, so anything reaching it means the check failed.
# plain exit rather than exit /b, the latter does not set the exit code of cmd /c
FAIL_IF_OUTPUT = findstr /r /c:"^" && exit 1 || exit 0
PLUGIN_DIR := $(APPDATA)/terraform.d/plugins
else
SHELL := /bin/bash
NULL_DEV := /dev/null
MKDIR_P  = mkdir -p "$1"
RM_RF    = rm -rf "$1"
COPY     = cp "$1" "$2"
SETENV   = $1
INC      = $(shell echo $$(($1+1)))
FAIL_IF_OUTPUT = grep '^' && exit 1 || exit 0
PLUGIN_DIR := $(HOME)/.terraform.d/plugins
endif

# no cgo in this provider, and setting it here avoids a per recipe env var prefix
export CGO_ENABLED := 0

# go knows the platform names terraform uses for its plugin directories, uname does not
KERNEL := $(shell go env GOOS)
ARCH   := $(shell go env GOARCH)
EXE    := $(if $(filter windows,$(KERNEL)),.exe,)

BIN_DIR    := bin
PROVIDER   := terraform-provider-proxmox
BINARY     := $(BIN_DIR)/$(PROVIDER)$(EXE)
GO_BIN     := $(shell go env GOPATH)/bin

# platforms the bundle target cross compiles for, as <goos>_<goarch>
BUNDLE_PLATFORMS := darwin_arm64 linux_amd64 windows_amd64

# split a <goos>_<goarch> platform into its parts
GOOS_OF   = $(word 1,$(subst _, ,$1))
GOARCH_OF = $(word 2,$(subst _, ,$1))
EXE_OF    = $(if $(filter windows,$(call GOOS_OF,$1)),.exe,)

# latest v-prefixed tag, falling back to v0.0.0 while the repo has no tags
VERSION_TAG := $(shell git describe --match "v*" --abbrev=0 --tags 2>$(NULL_DEV))
ifeq ($(strip $(VERSION_TAG)),)
VERSION_TAG := v0.0.0
endif

VERSION       := $(subst v,,$(VERSION_TAG))
VERSION_PARTS := $(subst ., ,$(VERSION))

MAJOR := $(word 1,$(VERSION_PARTS))
MINOR := $(word 2,$(VERSION_PARTS))
MICRO := $(word 3,$(VERSION_PARTS))

NEXT_MICRO     = $(call INC,$(MICRO))
DEV_VERSION    = $(MAJOR).$(MINOR).$(NEXT_MICRO)
DEV_NAMESPACE := $(PLUGIN_DIR)/localhost/telmate/proxmox
DEV_PLUGIN_DIR = $(DEV_NAMESPACE)/$(DEV_VERSION)/$(KERNEL)_$(ARCH)

# bundle writes straight into a terraform filesystem mirror, so it needs to be told where.
# BUNDLE_DIR is the version directory, ie the one holding the <goos>_<goarch> subdirectories.
ifneq ($(filter bundle bundle-%,$(MAKECMDGOALS)),)
ifeq ($(strip $(BUNDLE_DIR)),)
$(error BUNDLE_DIR is not set, expected the version directory of a terraform plugin mirror. \
Example: make bundle BUNDLE_DIR=D:/Work/keepsake-studio/terraform/layer_0/terraform.d/plugins/registry.keepsake.dev/mollstam/proxmox/0.1.0)
endif
endif

BUNDLE_TARGETS := $(BUNDLE_PLATFORMS:%=bundle-%)

.PHONY: all info fmt vet test build acctest install local-dev-install bundle clean
# these have to be a static pattern rule below rather than a plain pattern rule,
# make skips the implicit rule search for phony targets
.PHONY: $(BUNDLE_TARGETS)

all: build

# cmd.exe echoes quotes literally, so these messages avoid quoting by staying
# clear of characters both shells treat as operators
info:
	@echo Global info
	@echo kernel: $(KERNEL)
	@echo arch: $(ARCH)
	@echo dev version: $(DEV_VERSION)

fmt:
	@echo Checking code style
	@gofmt -l . | $(FAIL_IF_OUTPUT)

vet:
	@echo Vetting code
	@go vet ./...

test:
	@echo Testing code
	@go test -v ./...

build: clean
	@echo Building
	$(call MKDIR_P,$(BIN_DIR))
	go build -trimpath -o $(BINARY)
	@echo Built $(BINARY)

# the acceptance tests all share one PVE, and they assert on hardcoded VM IDs. a test
# that fails part way through can leave a guest behind that failed to delete, after which
# every later test finds VM 101 where it expected 100 and the real failure gets buried.
# -failfast stops at the first one, which is reliable here because no test calls t.Parallel
# and they all live in a single package, ie a single test binary.
# the suite runs well past go test's default 10m timeout, hence the explicit one.
ACCTEST_FLAGS := -failfast -timeout 60m

# to run only certain tests, run something of the form:  make acctest TESTARGS=-run=TestAccVMResource_CreateAndUpdateVLAN
# to see progress during the run, add -v:                make acctest TESTARGS=-v
acctest: build
	$(call SETENV,TF_ACC=1) go test $(ACCTEST_FLAGS) ./proxmox/... $(TESTARGS)

install: build
	$(call MKDIR_P,$(GO_BIN))
	$(call COPY,$(BINARY),$(GO_BIN)/terraform-provider-proxmox$(EXE))

local-dev-install: build
	@echo Installing $(DEV_VERSION) for $(KERNEL)_$(ARCH) into $(PLUGIN_DIR)
	$(call RM_RF,$(DEV_NAMESPACE))
	$(call MKDIR_P,$(DEV_PLUGIN_DIR))
	$(call COPY,$(BINARY),$(DEV_PLUGIN_DIR)/terraform-provider-proxmox$(EXE))

# cross compile every bundled platform into a terraform filesystem mirror, eg:
#   make bundle BUNDLE_DIR=.../terraform.d/plugins/registry.keepsake.dev/mollstam/proxmox/0.1.0
# override the platform set with:  make bundle BUNDLE_DIR=... BUNDLE_PLATFORMS=linux_amd64
bundle: $(BUNDLE_TARGETS)
	@echo Bundled $(BUNDLE_PLATFORMS) into $(BUNDLE_DIR)

$(BUNDLE_TARGETS): bundle-%:
	@echo Cross compiling $*
	$(call MKDIR_P,$(BUNDLE_DIR)/$*)
	$(call SETENV,GOOS=$(call GOOS_OF,$*)) $(call SETENV,GOARCH=$(call GOARCH_OF,$*)) go build -trimpath -o "$(BUNDLE_DIR)/$*/$(PROVIDER)$(call EXE_OF,$*)"

clean:
	@echo Cleaning
	$(call RM_RF,$(BIN_DIR))
