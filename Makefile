VERSION ?= 0.2.0
GO ?= go
LDFLAGS := -s -w -X weefleet/internal/app.Version=$(VERSION)
PLATFORMS := darwin-arm64 darwin-amd64 linux-arm64 linux-amd64

.PHONY: build test vet dist install clean

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/weefleet ./cmd/weefleet

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

dist:
	mkdir -p dist
	$(foreach p,$(PLATFORMS),GOOS=$(word 1,$(subst -, ,$(p))) GOARCH=$(word 2,$(subst -, ,$(p))) \
		$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o dist/weefleet-$(VERSION)-$(p) ./cmd/weefleet;)

install: build
	./bin/weefleet install

clean:
	rm -rf bin dist
