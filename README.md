# fairyringclient

`fairyringclient` is a sample script for interacting with [`fairyring`](https://github.com/FairBlock/fairyring).

## Generating keys for getting shares

1. Create keys directory

```bash
mkdir keys
```

2.Generate key using `ssh-keygen`

```bash
ssh-keygen -t rsa -b 2048 -m PEM -f ./keys/sk1.pem
```

3. Generate public key from the private key

```bash
ssh-keygen -f ./keys/sk1.pem -e -m pem > ./keys/pk1.pem
```

4. Gather all the public key and put them inside `keys/` and rename it as `pk{number}.pem` start the number from 1.

## Running client

First start the chain by navigating to `fairyring` directory.

```
ignite chain serve
```

Optionally add in `-r` flag to reset chain state, and `-v` to have verbose output.

Navigate to this repo and create `.env` file, you can look at the `.env.example` for example.

Then run the client by the following command:

```
go run main.go
```

to start the client.

This script first registers the validator, and then broadcasts new keyshares on each block.

Make sure to change the accounts to whatever account you wish to run.
