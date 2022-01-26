NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m

SERVICE_NAME=api-gateway

.PHONY: all format lint lint-chart build
all: format lint lint-chart build

run:
	@go run main.go

build:
	@echo "$(OK_COLOR)==> Building $(SERVICE_NAME)...$(NO_COLOR)"
	@go build -o bin/$(SERVICE_NAME) main.go

compile:
	@echo "$(OK_COLOR)==> Compiling $(SERVICE_NAME) for Linux x86-64...$(NO_COLOR)"
	@GOOS=linux GOARCH=amd64 go build -o bin/$(SERVICE_NAME)-linux-amd64 main.go

format:
	@echo "$(OK_COLOR)==> Formatting $(SERVICE_NAME)...$(NO_COLOR)"
	@go fmt

lint:
	@echo "$(OK_COLOR)==> Linting $(SERVICE_NAME)...$(NO_COLOR)"
	@golangci-lint run

# Helm

lint-chart:
	@echo "$(OK_COLOR)==> Linting helm chart of $(SERVICE_NAME)... $(NO_COLOR)"
	@helm lint -f ./chart/values.yaml -f ./chart/values-prod.yaml ./chart

render-chart:
	@echo "$(OK_COLOR)==> Rendering helm chart of $(SERVICE_NAME)... $(NO_COLOR)"
	@helm template --output-dir=.chart.rendered -f ./chart/values.yaml -f ./chart/values-prod.yaml ./chart