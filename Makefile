.PHONY: clean all release build

all: test build

# Build binaries
build: 
	hack/make-rules/build.sh $(WHAT)

# Run test
test: fmt vet
	go test ./pkg/... ./cmd/... -coverprofile cover.out

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

release:
	build/release-images.sh $(ARCH)

clean: 
	-rm -Rf _output
	-rm -Rf dockerbuild
