.PHONY: all clean lint

all: build/bin/hub-scrub

build/bin/hub-scrub: cmd/hub-scrub/main.go
	go build -o $@ $<

lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint run

clean:
	rm -rf build
