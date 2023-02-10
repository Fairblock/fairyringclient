# fairyringclient

`fairyringclient` is a sample script for interacting with [`fairyring`](https://github.com/FairBlock/fairyring).

## Running client

First start the chain by navigating to `fairyring` directory.

```
ignite chain serve
```

Optionally add in `-rv` flag to reset and have verbose output.

Navigate to this repo and run

```
go run main.go
```

to start the client.

This script first registers the validator, and then broadcasts new keyshares on each block.

Make sure to change the accounts to whatever account you wish to run.
