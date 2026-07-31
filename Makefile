BINARY := bin/sdlc-controls

.PHONY: build test lint run clean

build:
	CGO_ENABLED=0 go build -o $(BINARY) ./cmd/sdlc-controls

test:
	go test ./...

lint:
	go vet ./...

run: build
	./$(BINARY) $(ARGS)

clean:
	rm -rf bin dist
