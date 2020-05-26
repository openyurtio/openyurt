.PHONY: clean all

all: build

build: 
	hack/make-rules/build.sh $(WHAT)

clean: 
	-rm -Rf _output
