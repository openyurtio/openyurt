.PHONY: clean all release build

all: build

build: 
	hack/make-rules/build.sh $(WHAT)

release:
	build/release-images.sh

clean: 
	-rm -Rf _output
	-rm -Rf dockerbuild

