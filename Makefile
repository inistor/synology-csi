IMAGE ?= ghcr.io/inistor/synology-csi
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

ifeq ($(GOARCH),)
GOARCH:=amd64
endif
GOARM?=""
BUILD_ENV=CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) GOARM=$(GOARM)
BUILD_FLAGS="-s -w -extldflags \"-static\" -X github.com/inistor/synology-csi/pkg/driver.DriverVersion=$(VERSION)"

.PHONY: all
all: build

.PHONY: FORCE
FORCE: ;

.PHONY: build
build: bin/synology-csi-driver bin/synocli

bin:
	@mkdir -p $@

bin/synology-csi-driver: bin FORCE
	@echo "Compiling $@…"
	@$(BUILD_ENV) go build -v -ldflags $(BUILD_FLAGS) -o $@ ./

bin/synocli: bin FORCE
	@echo "Compiling $@…"
	@$(BUILD_ENV) go build -v -ldflags $(BUILD_FLAGS) -o $@ ./synocli

# Local image for testing; CI builds and publishes the multi-arch images.
.PHONY: docker-build
docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

.PHONY: test
test:
	go test ./pkg/...

# CSI sanity suite against a real DSM; see README.
.PHONY: sanity
sanity:
	go test -tags sanity -count=1 -v ./test/sanity/...

.PHONY: clean
clean:
	-rm -rf ./bin
