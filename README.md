# Prometheus metrics exporter for the Zyxel GS1200 series switches

The [Zyxel GS1200-5/GS1200-8](https://www.zyxel.com/products_services/5-Port-8-Port-Web-Managed-Gigabit-Switch-GS1200-5-GS1200-8/)
series of switches and their
[PoE enabled siblings](https://www.zyxel.com/nl/nl/products/switch/5-port-8-port-web-managed-poe-gigabit-switch-gs1200-poe-series)
are small and cheap managed switches, capable of handling 16Gbps across
5 or 8 ports, so say the specs.

Unfortunately the firmware does not implement SNMP or another standard for
collecting port metrics.

It does, however, display metrics in the web gui. These metrics are set in a
number of javascript files at uris like `/link_data.js`.

This program will automatically log in to the web gui, evaluate the scripts, and
output metrics for use by Prometheus.

## Usage

The program can be configured by setting environment variables prior to running:

| name              | required | description                            |
|-------------------|----------|----------------------------------------|
| `GS1200_ADDRESS`  | yes      | IP address of the GS1200               |
| `GS1200_PASSWORD` | yes      | Password to log on with                |
| `GS1200_PORT`     | no       | Port number to listen on, default 9934 |

Example:

```shell
$ go build
$ export GS1200_ADDRESS=192.168.1.3
$ export GS1200_PASSWORD=1234
$ ./gs1200-exporter
```

Or just use the argument flags:

```shell
$ ./gs1200-exporter --help
Usage of ./gs1200-exporter:
  -address string
        IP address or hostname of the GS1200 (default "192.168.1.3")
  -password string
        Password to log on to the GS1200 (default "********")
  -port string
        Port on which to expose metrics. (default "9934")
```

## Running with Docker

```shell
$ docker run \
    --detach \
    --name gs1200-exporter \
    --rm \
    --publish 9934:9934 \
    --env GS1200_ADDRESS=192.168.1.3 \
    --env GS1200_PASSWORD=1234 \
    ghcr.io/robinelfrink/gs1200-exporter:latest
```

## Running with Docker Compose

Example `docker-compose.yml`:

```yaml
version: '3'
services:
  gs1200-exporter:
    container_name: gs1200-exporter
    image: ghcr.io/robinelfrink/gs1200-exporter:latest
    environment:
      - GS1200_ADDRESS=192.168.1.3
      - GS1200_PASSWORD=1234
    ports:
      - 9934:9934
```

## NixOS Module

[![nixpkgs](https://repology.org/badge/version-for-repo/nix_unstable/gs1200-exporter.svg)](https://repology.org/project/gs1200-exporter/versions)

A NixOS module is available in nixpkgs for running gs1200-exporter as a systemd service.
```nix
services.gs1200-exporter = {
  enable = true;
  address = "192.168.1.3";
  port = 9934;
  passwordFile = "/run/secrets/gs1200-password";
};
```

All available options:
| Option | Required | Default | Description |
|--------|----------|---------|-------------|
| `enable` | yes | `false` | Enable the gs1200-exporter service |
| `address` | yes | | IP address or hostname of the GS1200 switch |
| `passwordFile` | no | `null` | Path to a file containing the password (recommended) |
| `port` | no | `9934` | Port on which to expose Prometheus metrics |
| `debug` | no | `false` | Enable debug logging |
| `verbose` | no | `false` | Enable verbose logging |
| `json` | no | `false` | Output logs in JSON format |

Logs are accessible via:
```shell
$ journalctl -u gs1200-exporter -f
```
> **Note:** `passwordFile` is recommended as it avoids storing the password in the Nix store. It is compatible with [sops-nix](https://github.com/Mic92/sops-nix) and [agenix](https://github.com/ryantm/agenix).

## Compatibility

This application has been tested with V2.00 firmwares up to V2.00.

