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
ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
REPO=github.com/RobustPerception/azure_metrics_exporter
LD_FLAGS="-w -s -extldflags -static"

export GOPATH=$(shell pwd)/gopath

.PHONY: all
all: bin/azure_metrics_exporter

gopath:
	$(Q)mkdir -p gopath/src/github.com/RobustPerception
	$(Q)ln -s $(ROOT_DIR) gopath/src/$(REPO)

GO_SOURCES := $(shell find . -type f -name "*.go")

bin/%: $(GO_SOURCES) | gopath
	$(Q)go build -o $@ -ldflags $(LD_FLAGS) $(REPO)

.PHONY: vendor
vendor:
	$(Q)glide update --strip-vendor
	$(Q)glide-vc --use-lock-file --no-tests --only-code

.PHONY: clean
clean:
	$(Q)rm -rf bin
