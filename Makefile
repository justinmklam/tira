# Makefile for lazyjira

.PHONY: build clean

build:
	GOTOOLCHAIN=local go build -o lazyjira ./cmd/lazyjira

clean:
	rm -f lazyjira
