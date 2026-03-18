# Makefile for lazyjira

.PHONY: build clean

build:
	go build -o lazyjira ./cmd/lazyjira

clean:
	rm -f lazyjira
