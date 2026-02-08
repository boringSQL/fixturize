.PHONY: build test test-all test-integration clean

build:
	go build -o bin/fixturize ./cmd/fixturize

test:
	go test ./fixturize/ -count=1

test-all: test test-integration

test-integration:
	go test -tags integration -v ./fixturize/ -count=1

clean:
	rm -rf bin/
