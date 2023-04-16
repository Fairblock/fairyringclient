# fairyringclient

`fairyringclient` is a client for submitting their keyshare to [`fairyring`](https://github.com/FairBlock/fairyring).

## Generate your identity key

1. Create keys directory

```bash
mkdir keys
```

2. Run `generate_keys.sh`

```bash
./generate_keys.sh {start_at} {end_at}
```

#### Examples

Let's say you would like to create two keys, from 1 to 2, enter the following command:

```bash
./generate_keys.sh 1 2
```

Let's say you would like to create only one keys, enter the following command:

```bash
./generate_keys.sh 1 1
```

This command will generate a public key & private key for you.

The public key is your identity for our API server to recognize you and to give you the correct key share and for the server to encrypt your keyshare with your public key.

The private key is for you to decrypt the keyshare you got from API server

## Setting up the client for testnet / mainnet

### Updating .env

These are the fields that would require update from the `.env.example`:

```
NODE_IP_ADDRESS=http://your_node_address
NODE_PORT=your_node_port

GRPC_IP_ADDRESS=your_node_address
GRPC_PORT=9090

# Only update this if your private key file index is not start from 1
PRIVATE_KEY_FILE_NAME_INDEX_START_AT=1

# Only update this if the default token denom is not FRT
DENOM=frt

# Put your private key here, separate with comma
VALIDATOR_PRIVATE_KEYS=private_key_1,private_key2,private_key3
```

### Prepare account for submitting keyshare to fairyring chain

The script `generate_keys.sh` will generate private key(s) for you, you can you the one it generated or use your own private key.

Make sure the account you are using already activated and have enough balance for sending transaction

Then, put your private key in `VALIDATOR_PRIVATE_KEYS` in `.env`. If you are using your own account.

Make sure you have te same number of public keys & private keys in the `keys` directory and the number of private keys in `VALIDATOR_PRIVATE_KEYS`

## Setting up the client for local testnet

### 1. Start the chain by navigating to `fairyring` directory.

```
ignite chain serve
```

Optionally add in `-r` flag to reset chain state, and `-v` to have verbose output. `-c` to specific a config file

The following command is recommended  when running a local fairyring chain for testing purpose

```
ignite chain serve -c fairyring.yml -v
```
** Add the `-r` flag if you would like to reset chain state

### 2. Modify client's config file

Navigate to this repo and create `.env` file, you can look at the `.env.example` for example.

What usually need to be updated in the `.env` is the following:

```
NODE_IP_ADDRESS=http://change_to_your_node_ip
NODE_PORT=change_to_your_node_port

# Update to a correct total validator number for manager to setup correctly
TOTAL_VALIDATOR_NUM=

# Update IS_MANAGER to true
IS_MANAGER=true

MASTER_PRIVATE_KEY=
```

For the master private key, if you are running fairyring with the recommended command, you can use the following command to export the private key:

`fairyringd keys export bob --unsafe --unarmored-hex`

You will also need to update the `DENOM` in `main.go line 36` to `token`

What the master private key does, is it will load the account and send some tokens from the master to all the accounts in `VALIDATOR_PRIVATE_KEYS`

## Start the client

Then run the client by the following command:

```
go run main.go
```

The client will look for `VALIDATOR_PRIVATE_KEYS` in `.env`, and will automatically run the same number of client to submit keyshares.

<small>* Separate your private keys with comma.</small>
