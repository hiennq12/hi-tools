BINARIES := hi-brains hi-llm
BIN_DIR  := bin
GOBIN    := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

.PHONY: all build install test vet clean $(BINARIES)

all: build

build: $(BINARIES)

$(BINARIES):
	go build -o $(BIN_DIR)/$@ ./cmd/$@

install: $(BINARIES)
	@mkdir -p "$(GOBIN)"
	@for b in $(BINARIES); do cp $(BIN_DIR)/$$b "$(GOBIN)/$$b"; echo "installed $$b -> $(GOBIN)/$$b"; done

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf $(BIN_DIR)
