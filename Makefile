.PHONY: build run test lint clean

build:
	go build -o xray-sub-rotation ./cmd/xray-sub-rotation/

run:
	go run ./cmd/xray-sub-rotation/

test:
	go test ./... -v

lint:
	golangci-lint run ./...

clean:
	rm -f xray-sub-rotation
