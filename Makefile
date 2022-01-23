NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m

SERVICE_NAME=api-gateway

.PHONY: all lint build
all: lint build

run:
	@go run main.go

build:
	@echo "$(OK_COLOR)==> Building $(SERVICE_NAME)...$(NO_COLOR)"
	@go build -o bin/$(SERVICE_NAME) main.go

compile:
	@echo "$(OK_COLOR)==> Compiling $(SERVICE_NAME) for Linux x86-64...$(NO_COLOR)"
	@GOOS=linux GOARCH=amd64 go build -o bin/$(SERVICE_NAME)-linux-amd64 main.go

lint:
	@echo "$(OK_COLOR)==> Linting $(SERVICE_NAME)...$(NO_COLOR)"
	@golangci-lint run