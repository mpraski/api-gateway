# API Gateway

A very simple HTTP reverse proxy with basic request routing and authentication strategy.

## Purpose

This little proxy is designed to sit between the public L7 HTTP/HTTPS load balancer and the services in the private network. It provides basic routing and request authentication via OAuth 2.0 token introspection. You'll only need to supply the URL of the token introspection endpoint provided by the OAuth 2.0 Server, for instance [ORY Hydra](https://github.com/ory/hydra).

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
