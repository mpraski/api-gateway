# API Gateway

An HTTP reverse proxy with support for:
* basic request routing
* custom request authentication
* rate limiting
* CORS configuration

## Purpose

This little proxy is designed to sit between the public L7 HTTP/HTTPS load balancer and the services in a private network. It is successfully used in production at [BlueHealth](https://github.com/blue-health), where it routes incoming traffic from GCP Cloud Load Balancer to respective Kube service withing GKE cluster. We chose to build it to accomodate our custom authenticate strategy and provide a minimum set of features an API gateway should have.

## Builing

To build for you local architecture:

```bash
make build
```

To build for Linux x86-68:

```bash
make compile
```

## Usage

To run with example config:

```bash
API_GATEWAY_CONFIG=$(cat example/config.yaml) make run
```

## Authors

- [Marcin Praski](https://github.com/mpraski)
