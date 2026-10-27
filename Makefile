VERSION ?= 0.9.8
GO ?= go
LDFLAGS := -s -w -X weeco/internal/commands.Version=$(VERSION)
PLATFORMS := darwin-arm64 darwin-amd64 linux-arm64 linux-amd64

.PHONY: build test vet dist install clean

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/weeco ./cmd/weeco

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

dist:
	mkdir -p dist
	$(foreach p,$(PLATFORMS),GOOS=$(word 1,$(subst -, ,$(p))) GOARCH=$(word 2,$(subst -, ,$(p))) \
		$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o dist/weeco-$(VERSION)-$(p) ./cmd/weeco;)

install: build
	./bin/weeco install

clean:
	rm -rf bin dist
