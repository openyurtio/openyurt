.PHONY: clean all release

all: build

build: 
	hack/make-rules/build.sh $(WHAT)

release:
	build/release-images.sh

clean: 
	-rm -Rf _output
