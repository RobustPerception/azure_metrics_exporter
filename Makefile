# This Makefile has been adapted from the one used by coreos/locksmith.

# kernel-style V=1 build verbosity
ifeq ("$(origin V)", "command line")
	BUILD_VERBOSE = $(V)
endif

ifeq ($(BUILD_VERBOSE),1)
	Q =
else
	Q = @
endif

GO ?= GO15VENDOREXPERIMENT=1 go
GOPATH := $(firstword $(subst :, ,$(shell $(GO) env GOPATH)))
PROMU ?= $(GOPATH)/bin/promu
# This is not an autoconf style prefix, it is the path where promu
# will place binaries
PREFIX ?= $(shell pwd)/bin

.PHONY: all build clean promu $(PROMU)

all: build

build: $(PROMU)
	$(Q)$(PROMU) build --prefix $(PREFIX)


$(GOPATH)/bin/promu promu:
	$(Q)GOOS= GOARCH= $(GO) get -u github.com/prometheus/promu

clean:
	$(Q)rm -rf bin
