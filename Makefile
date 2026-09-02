BINARY := bin/sdlc-controls

.PHONY: build test lint run demo clean

build:
	CGO_ENABLED=0 go build -o $(BINARY) ./cmd/sdlc-controls

test:
	go test ./...

lint:
	go vet ./...

run: build
	./$(BINARY) $(ARGS)

demo: build
	./scripts/demo.sh

clean:
	rm -rf bin dist
