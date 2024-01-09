FROM gcr.io/oss-fuzz-base/base-builder-go

COPY ./ $GOPATH/src/github.com/openyurtio/openyurt/
COPY ./test/fuzz/oss_fuzz_build.sh $SRC/build.sh

WORKDIR $SRC