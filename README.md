# Prometheus metrics exporter for the Zyxel GS1200-8

The [Zyxel GS1200-8](https://www.zyxel.com/products_services/5-Port-8-Port-Web-Managed-Gigabit-Switch-GS1200-5-GS1200-8/)
is a small and cheap managed switch, capable of handling 16Gbps across 8 ports,
so say the specs.

Unfortunately the firmware does not implement SNMP or another standard for
collecting port metrics.

It does, however, display metrics in the web gui. These metrics are set in a
javascript file at the uri `/link_data.js`.

This script will automatically log in to the web gui, evaluate the script, and
output metrics for use by Prometheus.

## Usage

The program can be configured by setting environment variables prior to running:

| name              | required | description                            |
|-------------------|----------|----------------------------------------|
| `GS1200_ADDRESS`  | yes      | IP address of the GS1200-8             |
| `GS1200_PASSWORD` | yes      | Password to log on with                |
| `GS1200_PORT`     | no       | Port number to listen on, default 9707 |

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
        Port on which to expose metrics. (default "9707")
```

## Running with Docker

```shell
$ docker run \
    --detach \
    --name gs1200-exporter \
    --rm \
    --publish 9707:9707 \
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
      - 9707:9707
```
