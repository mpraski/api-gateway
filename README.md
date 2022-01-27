# API Gateway

A very simple HTTP reverse proxy with built-in JWT verification and basic routing.

## Purpose

This little proxy is designed to sit between the public L7 HTTP/HTTPS load balancer and the services in the private network. It provides basic routing and request authentication via JWT.

## Builing

To build for you local architecture:

```bash
make build
```

To build for Linux x86-68:

```bash
make compile
```

To run with example config:

```bash
API_GATEWAY_CONFIG=$(cat example/config.yaml) make run
```

## Authors

- [Marcin Praski](https://github.com/mpraski)
