# A simple web network of public APIs

## To run

To run a query against an index (a demo index is provided at `./demo/sample_index.json`):

```bash
go run ./cmd/search/ --query "I want to buy a domain" --index ./demo/sample_index.json
```

To build an index (a demo set of API specs is provided at `./demo/public_api_specs`):

```bash
go run ./cmd/index/ --specs-path ./demo/public_api_specs --output-path ./new_index.json --max-concurrency 8
```

To build a browser and run a search query against the index (a demo content file is provided at `./demo/sample_content.json`):

```bash
go run ./cmd/browser --content demo/sample_content.json --query "I want to buy a domain" --max-concurrency 16
```
