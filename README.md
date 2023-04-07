# API Gateway

A very simple HTTP reverse proxy with basic request routing and authentication strategy.

## Purpose

This little proxy is designed to sit between the public L7 HTTP/HTTPS load balancer and the services in the private network. It provides basic routing and request authentication via a custom bearer token strategy.

This project is used with success in production at [https://github.com/blue-health](blue-health), where it routes incoming traffic from GCP Cloud Load Balancer to respective Kube service withing GKE cluster.

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
