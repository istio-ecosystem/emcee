# mccli

_mccli_ is a small command line interface that currently outputs OpenAPI/Swagger for ServiceExposures.

## usage

To print the exposures as OpenAPI:

``` bash
cd mccli/cmd
go run main.go [--context <ctx>] [--namespace <ns>]
```

To start a web server that returns the exposures as OpenAPI:

``` bash
cd mccli/server
go run main.go [--context <ctx>] [--namespace <ns>] [--port <port>]
```
