.PHONY: build

build:
	go build -v -o ./execs/run_arbitrage_robot ./cmd/arbitrage
	go build -v -o ./execs/start ./cmd/start
	go build -v -o ./execs/update ./cmd/update
	go build -v -o ./execs/stop ./cmd/stop

.DEFAULT_GOAL := build
