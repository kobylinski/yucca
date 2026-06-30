.PHONY: build install ui daemon service clean

VERSION := dev-$(shell date +%Y%m%d-%H%M%S)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
GOBIN := $(shell go env GOPATH)/bin

ui:
	cd ui && pnpm install && pnpm build
	rm -rf internal/daemon/ui_dist
	cp -r ui/build internal/daemon/ui_dist

build: ui
	go build $(LDFLAGS) -o yucca ./cmd/yucca

install: ui
	go install $(LDFLAGS) ./cmd/yucca
	@echo "Installing/refreshing the managed daemon service..."
	-$(GOBIN)/yucca daemon install

# Install (or refresh) the OS-managed daemon service without rebuilding.
service:
	$(GOBIN)/yucca daemon install

daemon: build
	./yucca daemon --port 9777

clean:
	rm -f yucca
	rm -rf ui/build internal/daemon/ui_dist
