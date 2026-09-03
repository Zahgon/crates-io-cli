EXECUTABLE = crates

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

$(EXECUTABLE): ## Build the binary
	go build -o $(EXECUTABLE) .

##@ Testing

tests: check unit-tests ## Run all tests we have

unit-tests: ## Run all unit tests, including the recorded-response differential
	go test ./...

check: ## Vet and format checks
	go vet ./...
	gofmt -l .

race: ## Run the suite under the race detector
	go test -race ./...

clean:
	rm -f $(EXECUTABLE)
