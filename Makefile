.PHONY: lint test build

bin/custom-gcl: .custom-gcl.yml $(wildcard tools/commentlint/*.go)
	golangci-lint custom

lint: bin/custom-gcl
	./bin/custom-gcl run ./...

test:
	go test ./...

build:
	go build -o bin/tektite ./cmd/tektite
	ln -sf tektite bin/tk
