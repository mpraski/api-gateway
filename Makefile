NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m

SERVICE_NAME=api-gateway-srv

.PHONY: all format build
all: format build

run:
	@go run main.go

build: test
	@echo "$(OK_COLOR)==> Building $(SERVICE_NAME)...$(NO_COLOR)"
	@go build -o bin/$(SERVICE_NAME) main.go

compile:
	@echo "$(OK_COLOR)==> Compiling $(SERVICE_NAME) for Linux x86-64...$(NO_COLOR)"
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/$(SERVICE_NAME)-linux-amd64 main.go

test: lint
	@echo "$(OK_COLOR)==> Testing $(SERVICE_NAME)...$(NO_COLOR)"
	@go test

lint: 
	@echo "$(OK_COLOR)==> Linting $(SERVICE_NAME)...$(NO_COLOR)"
	@golangci-lint run

format:
	@echo "$(OK_COLOR)==> Formatting $(SERVICE_NAME)...$(NO_COLOR)"
	@go fmt

# Helm

lint-chart:
	@echo "$(OK_COLOR)==> Linting helm chart of $(SERVICE_NAME)... $(NO_COLOR)"
	@helm lint -f ./chart/values.yaml -f ./chart/values-develop.yaml ./chart
	@helm lint -f ./chart/values.yaml -f ./chart/values-production.yaml ./chart
 
render-chart:
	@echo "$(OK_COLOR)==> Rendering helm chart of $(SERVICE_NAME)... $(NO_COLOR)"
	@helm template --output-dir=.chart.rendered -f ./chart/values.yaml -f ./chart/values-develop.yaml api-gateway-srv ./chart
