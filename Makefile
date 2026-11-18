.DEFAULT_GOAL := build

TEST_DATABASE_URL ?=
export TEST_DATABASE_URL

.PHONY: build
build:
	CGO_ENABLED=0 go build -trimpath -o bin/print-dis ./cmd/print-dis

# Local development: DEV_USER sign-in, files on disk. Needs `make db`.
.PHONY: run
run: build
	DATABASE_URL=$${DATABASE_URL:-postgres://postgres:pg@localhost:55432/printdis?sslmode=disable} \
	BASE_URL=http://localhost:8080 DEV_USER=$${DEV_USER:-dev@localhost} \
	FILES_BACKEND=$${FILES_BACKEND:-dir} FILES_DIR=$${FILES_DIR:-./.files} \
	./bin/print-dis serve

# Throwaway Postgres for tests and `make run`, on port 55432. Uses podman or
# docker if one is running, else local Postgres binaries; see scripts/devdb.sh.
.PHONY: db
db:
	./scripts/devdb.sh start

# Stop it and throw its data away.
.PHONY: db-stop
db-stop:
	./scripts/devdb.sh stop

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: test
test:
	go test -race -cover ./...

.PHONY: check
check: vet lint test

# Fetch the htmx and SortableJS versions pinned in internal/web/thirdparty
# from npm into internal/web/static, checked against npm's integrity hashes.
.PHONY: vendor-js
vendor-js:
	go run ./internal/web/thirdparty/fetch

# Regenerate the README images in docs/images from made-up data. Needs
# `make db` and chromium on PATH.
.PHONY: screenshots
screenshots:
	SCREENSHOTS=1 TEST_DATABASE_URL=$${TEST_DATABASE_URL:-postgres://postgres:pg@localhost:55432/printdis?sslmode=disable} \
	go test -run '^TestScreenshots$$' -count=1 -v ./internal/web/
	@if command -v oxipng >/dev/null; then oxipng -q -o max --strip safe docs/images/*.png; \
	else echo "oxipng not found; PNGs left uncompressed"; fi

.PHONY: clean
clean:
	rm -rf bin .files
