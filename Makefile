.PHONY: build test test-integration clean

build:
	go build -o bin/fixturize ./cmd/fixturize

test-integration:
	go test -tags integration -v ./fixturize/ -count=1

clean:
	rm -rf bin/
