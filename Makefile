.PHONY: all build test clean run-server run-agent release release-snapshot check-release help embed-binaries tag

BINARY_NAME=maek
MAIN_SRC=./cmd/maek

all: build

embed-binaries:
	@./hack/embed-binaries.sh

build: embed-binaries
	go build -o $(BINARY_NAME) $(MAIN_SRC)

test:
	go test -v -count=1 ./...

clean:
	rm -rf $(BINARY_NAME) dist internal/server/static/bin/maek_*

check-release:
	go tool goreleaser check

release-snapshot:
	go tool goreleaser release --snapshot --clean

# Always performs a clean build (including embedded binaries), then
# interactively prompts for major/minor/patch bump, creates git tag, pushes to remote, and runs GoReleaser
release: clean
	@$(MAKE) --no-print-directory embed-binaries
	@./hack/release.sh $(if $(VERSION),--version=$(VERSION),) $(if $(BUMP),--bump=$(BUMP),)

# Create and push next tag interactively without running GoReleaser locally
tag:
	@./hack/release.sh --tag-only $(if $(VERSION),--version=$(VERSION),) $(if $(BUMP),--bump=$(BUMP),)

run-server:
	go run $(MAIN_SRC) server -p 8080

run-agent:
	go run $(MAIN_SRC) agent -s ws://127.0.0.1:8080 -t http://localhost:3000

docker-build:
	docker build -t $(BINARY_NAME):latest .

docker-run:
	docker run --rm -p 8080:8080 $(BINARY_NAME):latest

help:
	@echo "Available targets:"
	@echo "  make build             - Build the maek binary"
	@echo "  make test              - Run all unit and integration tests"
	@echo "  make release           - Clean build (incl. embedded binaries), bump git tag, push, and execute release"
	@echo "                           Options: VERSION=vX.Y.Z, BUMP=patch|minor|major"
	@echo "  make tag               - Automatically creates and pushes next git tag only"
	@echo "  make release-snapshot  - Build snapshot release artifacts locally in ./dist without publishing"
	@echo "  make check-release     - Validate .goreleaser.yaml configuration"
	@echo "  make docker-build      - Build Docker container image"
	@echo "  make docker-run        - Run maek server container on port 8080"
	@echo "  make clean             - Remove binary and dist directory"
