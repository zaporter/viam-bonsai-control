BUILD_CHANNEL?=local
OS=$(shell uname)
VERSION=v1.12.0
GIT_REVISION = $(shell git rev-parse HEAD | tr -d '\n')
TAG_VERSION?=$(shell git tag --points-at | sort -Vr | head -n1)
CGO_LDFLAGS=""
GO_BUILD_LDFLAGS = -ldflags "-extldflags=-static -X 'main.Version=${TAG_VERSION}' -X 'main.GitRevision=${GIT_REVISION}'"
TOOL_BIN = etc/gotools/$(shell uname -s)-$(shell uname -m)

.PHONY: default
default: build-module

.PHONY: tool-install
tool-install:
	GOBIN=`pwd`/$(TOOL_BIN) go install \
		github.com/edaniels/golinters/cmd/combined \
		github.com/golangci/golangci-lint/cmd/golangci-lint \
		github.com/AlekSi/gocov-xml \
		github.com/axw/gocov/gocov \
		gotest.tools/gotestsum \
		github.com/rhysd/actionlint/cmd/actionlint

.PHONY: lint
lint: tool-install
	go mod tidy
	export pkgs="`go list -f '{{.Dir}}' ./... | grep -v /proto/`" && echo "$$pkgs" | xargs go vet -vettool=$(TOOL_BIN)/combined
	GOGC=50 $(TOOL_BIN)/golangci-lint run -v --fix --config=./golangci.yaml

.PHONY: test
test:
	go test -v -coverprofile=coverage.txt -covermode=atomic ./...

.PHONY: build
build: 
	mkdir -p bin && CGO_ENABLED=0 CGO_LDFLAGS=${CGO_LDFLAGS} go build $(GO_BUILD_LDFLAGS) -o bin/module module/main.go

.PHONY: package
package: build
	mkdir -p build && tar -czf build/module.tar.gz ./bin/module

.PHONY: clean
clean: 
	rm -rf bin

