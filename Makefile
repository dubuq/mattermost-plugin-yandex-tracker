PLUGIN_ID ?= $(shell cat plugin.json | grep '"id"' | sed 's/.*"id": "\(.*\)".*/\1/')
PLUGIN_VERSION ?= $(shell cat plugin.json | grep '"version"' | sed 's/.*"version": "\(.*\)".*/\1/')
BUNDLE_NAME ?= $(PLUGIN_ID)-$(PLUGIN_VERSION).tar.gz

MM_SERVICESETTINGS_SITEURL ?= http://localhost:8065
MM_ADMIN_USERNAME ?= admin
MM_ADMIN_PASSWORD ?=

# Optional: create a .env file (gitignored) to set MM_ADMIN_USERNAME and MM_ADMIN_PASSWORD locally.
-include .env

## Build server binary for all target platforms.
##
## The Go toolchain is pinned to 1.23. Do NOT bump it without re-verifying at
## runtime — Go 1.25+ fatally crashes this plugin inside Mattermost during
## outbound HTTPS to the Tracker API. Two distinct fatals were reproduced on a
## Go 1.26.4 build (2026-07), both in the net/http -> crypto/tls path:
##   - fatal error: select on synctest channel from outside bubble
##   - panic: runtime error: growslice: len out of range   (crypto/x509 CertPool, TLS handshake)
## MM health-check-restarts the plugin in a loop, so cards and card actions fail
## intermittently.
##
## We attempted the upgrade deliberately (Go 1.26 + server/public v0.4.3 + the
## CVE-patched x/net and grpc) to clear govulncheck findings. It didn't work:
## those dependency versions REQUIRE Go 1.25+, which is exactly what reintroduces
## the crash — stability and the newer deps are mutually exclusive. So we stay on
## 1.23 for stability and knowingly accept that the x/net / grpc / server-public
## dependency CVEs remain unpatched on the shipped build. Revisit when a Go release
## fixes the net/http+synctest fatal, or when MM's plugin runtime tolerates a newer Go.
GO_BUILD_IMAGE ?= golang:1.23-alpine
.PHONY: server
server:
	mkdir -p server/dist
	docker run --rm \
		-v "$(CURDIR):/src" \
		-w /src/server \
		-e CGO_ENABLED=0 \
		$(GO_BUILD_IMAGE) sh -c "\
			GOOS=linux   GOARCH=amd64 go build -trimpath -o dist/plugin-linux-amd64       . && \
			GOOS=linux   GOARCH=arm64 go build -trimpath -o dist/plugin-linux-arm64       . && \
			GOOS=darwin  GOARCH=amd64 go build -trimpath -o dist/plugin-darwin-amd64      . && \
			GOOS=darwin  GOARCH=arm64 go build -trimpath -o dist/plugin-darwin-arm64      . && \
			GOOS=windows GOARCH=amd64 go build -trimpath -o dist/plugin-windows-amd64.exe ."

## Build webapp bundle.
.PHONY: webapp
webapp:
	cd webapp && npm install && npm run build

## Build everything.
.PHONY: build
build: server webapp

## Bundle plugin into a tar.gz ready for MM upload.
.PHONY: bundle
bundle: build
	rm -rf dist
	mkdir -p dist
	cp plugin.json dist/
	mkdir -p dist/server
	cp -r server/dist dist/server/dist
	mkdir -p dist/webapp
	cp -r webapp/dist dist/webapp/dist
	cd dist && tar -czf ../$(BUNDLE_NAME) plugin.json server webapp
	rm -rf dist
	@echo "Bundle: $(BUNDLE_NAME)"

## Deploy plugin to a running MM instance via API.
## Requires MM_SERVICESETTINGS_SITEURL and MM_ADMIN_PASSWORD to be set.
.PHONY: deploy
deploy: bundle
	@if [ -z "$(MM_ADMIN_PASSWORD)" ]; then \
		echo "MM_ADMIN_PASSWORD is not set. Usage: make deploy MM_ADMIN_PASSWORD=yourpassword"; \
		exit 1; \
	fi
	@TOKEN=$$(curl -s -X POST "$(MM_SERVICESETTINGS_SITEURL)/api/v4/users/login" \
		-H "Content-Type: application/json" \
		-d '{"login_id":"$(MM_ADMIN_USERNAME)","password":"$(MM_ADMIN_PASSWORD)"}' \
		-i | grep -i "^token:" | awk '{print $$2}' | tr -d '\r'); \
	if [ -z "$$TOKEN" ]; then \
		echo "Login failed — check MM_ADMIN_USERNAME and MM_ADMIN_PASSWORD"; \
		exit 1; \
	fi; \
	RESULT=$$(curl -s -o /dev/stderr -w "%{http_code}" -X POST "$(MM_SERVICESETTINGS_SITEURL)/api/v4/plugins" \
		-H "Authorization: Bearer $$TOKEN" \
		-F "plugin=@$(BUNDLE_NAME)" \
		-F "force=true"); \
	if [ "$$RESULT" = "200" ] || [ "$$RESULT" = "201" ]; then \
		echo "Deployed $(BUNDLE_NAME)"; \
	else \
		echo "Upload failed (HTTP $$RESULT)"; \
		exit 1; \
	fi

## Run Go tests.
.PHONY: test
test:
	cd server && go test ./...

## Run go vet and verify webapp types.
.PHONY: check
check:
	cd server && go vet ./...
	cd webapp && npx tsc --noEmit

## Remove all build artifacts.
.PHONY: clean
clean:
	rm -rf server/dist webapp/dist dist *.tar.gz

.DEFAULT_GOAL := build
