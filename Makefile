APP      := h2-ui
GOFLAGS  := -trimpath -ldflags "-s -w"

.PHONY: all build test bdd smoke coverage run clean release

all: test bdd build

build:
	go build $(GOFLAGS) -o $(APP) ./cmd/h2-ui

test:
	go test -race -coverprofile=coverage.out ./...

bdd:
	go test ./features/...

smoke: build
	bash scripts/smoke.sh ./$(APP)

coverage: test
	go tool cover -html=coverage.out -o coverage.html
	@echo open coverage.html

run: build
	H2UI_ADDR=127.0.0.1:2053 H2UI_DATA=/tmp/h2ui H2UI_HYCONF=/tmp/h2ui/hy.yaml \
	H2UI_ADMIN_USER=admin H2UI_ADMIN_PASS=admin ./$(APP)

release:
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o dist/$(APP) ./cmd/h2-ui && \
	  cd dist && tar czf $(APP)-linux-amd64.tar.gz $(APP) && rm $(APP)
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) -o dist/$(APP) ./cmd/h2-ui && \
	  cd dist && tar czf $(APP)-linux-arm64.tar.gz $(APP) && rm $(APP)

clean:
	rm -f $(APP) coverage.out coverage.html
	rm -rf dist
