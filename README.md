# A simple web network of APIs

A mini world-wide web of machines that serve public API specs rather than rendered web pages.

## Synthetic web

First, start the synthetic web:

```bash
go run ./cmd/www/main.go --servers ./synthetic_servers_v2 --max-num-servers 5
```

For this demo, we'll only spin up 5 servers. See `./endpoints.json` for the list of local servers.

Then, build an index of the synthetic web:

```bash
go run ./cmd/index --endpoints-path ./synthetic_servers_v2/endpoints.json --max-concurrency 16 --output-path ./synthetic-web-index.json
```

To run an agent on the synthetic web:

```bash
go run ./cmd/agent --search-index ./synthetic-web-index.json --query "Where's the nearest amc theater" --max-concurrency 16
```

### Search

To run a query against a search index:

```bash
go run ./cmd/search --query "Check my next allstate insurance checkup" --index ./demo/test-index.json
```

You can use the demo index in `./demo/test-index.json` or build your own search index.
