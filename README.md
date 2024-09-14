# A simple web network of APIs

A mini world-wide web of machines that serve public API specs rather than rendered web pages.

## To run

To run an agent on the new web network:

```bash
go run ./cmd/agent --content ./demo/sample_content.json --query "I want to buy a domain" --max-concurrency 16
```

To build a browser and run a search query against the index:

```bash
go run ./cmd/browser --content demo/sample_content.json --query "I want to buy a domain" --max-concurrency 16
```

To build an index:

```bash
go run ./cmd/index/ --specs-path ./demo/public_api_specs --output-path ./new_index.json --max-concurrency 8
```

To run a query against an index:

```bash
go run ./cmd/search/ --query "I want to buy a domain" --index ./demo/sample_index.json
```
