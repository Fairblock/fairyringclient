# fairyringclient

`fairyringclient` is a client for submitting their keyshare to [`fairyring`](https://github.com/FairBlock/fairyring).

## Building the client

This command will build the project to an executable in this directory

```bash
go build
```

If you would like to have the executable in `GOPATH`

```bash
go install
```

## Upgrading from the old fairyringclient to v0.2.0

### Initializing the config

Initialize the FairyRing Client by the following command,
It will create a config directory under your `$HOME` directory: `$HOME/.fairyringclient`.

```bash
fairyringclient config init
```

After initializing the config directory, 
If your node is not running on localhost / the GRPC port is not `9090` / the tendermint port is not `26657`,
Run the following command to update the config:

```bash
fairyringclient config --ip 'node-ip' --port 'tendermint port' --grpc-port 'grpc port'
```

For example if your node endpoint is on `192.168.1.100` and the tendermint port is updated to `26666`,
then you can run the following command to update the ip & port:

```bash
fairyringclient config update --ip "192.168.1.100" --port 26666
```

After upgrading the config, run the following command to show your config to make sure the config is correct:

```bash
fairyringclient config show
```

Here is an example output of the command:

```
> fairyringclient config show
Using config file: /Users/fairblock/.fairyringclient/config.yml
GRPC Endpoint: 192.168.1.100:9090
FairyRing Node Endpoint: http://192.168.1.100:26666
Chain ID: fairytest-3
Chain Denom: ufairy
InvalidSharePauseThreshold: 5
MetricsPort: 2222
Share API Url: https://7d3q6i0uk2.execute-api.us-east-1.amazonaws.com
```

---

### Adding the Cosmos key

You can add the key by the following command:

```bash
fairyringclient keys add "private key in hex"
```

Your cosmos key should be located inside the `.env` file under `VALIDATOR_PRIVATE_KEYS=` of last FairyRing Client.

Assuming you are using the same validator address in the new testnet `fairytest-2`, you can add that private key in the `.env` file to the client.

Here is an example output:

```bash
> fairyringclient keys add f5c691d4b53ec8c3a3ad35e88525f9b8f33d307b3414c93f1b856265409a3a04
Using config file: /Users/fairblock/.fairyringclient/config.yml
Successfully added cosmos private key to config!
```

Then you can see all the private key added to the client by following command:

```bash
> fairyringclient keys list
Using config file: /Users/fairblock/.fairyringclient/config.yml
Found total 1 private keys in config file
[0] f5c691d4b53ec8c3a3ad35e88525f9b8f33d307b3414c93f1b856265409a3a04
```

**The [0] at the beginning of the key is the index of your key, which is used to delete the key**

If you would like to remove the private key:

```bash
fairyringclient keys remove "private key index"
```

```bash
> fairyringclient keys remove 0
Using config file: /Users/fairblock/.fairyringclient/config.yml
Successfully removed specified cosmos private key in config!

> fairyringclient keys list
Using config file: /Users/fairblock/.fairyringclient/config.yml
Found total 0 private keys in config file
```

---

## Setting up the client for the first time


### Initializing the config

Initialize the FairyRing Client by the following command,
It will create a config directory under your `$HOME` directory: `$HOME/.fairyringclient`.

```bash
fairyringclient config init
```

After initializing the config directory,
If your node is not running on localhost / the GRPC port is not `9090` / the tendermint port is not `26657`,
Run the following command to update the config:

```bash
fairyringclient config --ip 'node-ip' --port 'tendermint port' --grpc-port 'grpc port'
```

For example if your node endpoint is on `192.168.1.100` and the tendermint port is updated to `26666`,
then you can run the following command to update the ip & port:

```bash
fairyringclient config update --ip "192.168.1.100" --port 26666
```

After upgrading the config, run the following command to show your config to make sure the config is correct:

```bash
fairyringclient config show
```

Here is an example output of the command:

```
> fairyringclient config show
Using config file: /Users/fairblock/.fairyringclient/config.yml
GRPC Endpoint: 192.168.1.100:9090
FairyRing Node Endpoint: http://192.168.1.100:26666
Chain ID: fairytest-3
Chain Denom: ufairy
InvalidSharePauseThreshold: 5
MetricsPort: 2222
Share API Url: https://7d3q6i0uk2.execute-api.us-east-1.amazonaws.com
```

---

### Adding the Cosmos key

You can add your validator private key by the following command:

```bash
fairyringclient keys add "private key in hex"
```

**The private key should be the validator address that is staking on fairyring chain**

Example: 

```bash
> fairyringclient keys add f5c691d4b53ec8c3a3ad35e88525f9b8f33d307b3414c93f1b856265409a3a04
Using config file: /Users/fairblock/.fairyringclient/config.yml
Successfully added cosmos private key to config!
```

Then you can see all the private key added to the client by following command:

```bash
> fairyringclient keys list
Using config file: /Users/fairblock/.fairyringclient/config.yml
Found total 1 private keys in config file
[0] f5c691d4b53ec8c3a3ad35e88525f9b8f33d307b3414c93f1b856265409a3a04
```

**The [0] at the beginning of the key is the index of your key, which is used to delete the key**

If you would like to remove the private key:

```bash
fairyringclient keys remove "private key index"
```

```bash
> fairyringclient keys remove 0
Using config file: /Users/fairblock/.fairyringclient/config.yml
Successfully removed specified cosmos private key in config!

> fairyringclient keys list
Using config file: /Users/fairblock/.fairyringclient/config.yml
Found total 0 private keys in config file
```

---

### Address Delegation

You can now delegate another address to submit the key share for you. 
Your address need to be a validator in the `keyshare` module in order to do this.

You will also need the private key for your validator account set in the client config before running this command

After delegating, you can remove the validator's private key and add the delegated address private key to the client instead

#### Authorizing an address

```bash
fairyringclient delegate add [address]
```

#### Removing the authorized address

```bash
fairyringclient delegate remove [address]
```

**Make sure the account you are delegating is already activated and have enough balance for sending transaction**

## Starting the client

You can start the client by the following command, if will automatically use the config under
`$HOME/.fairyringclient/config.yml` directory.

```
fairyringclient start
```

If you get this error `fairyringclient: command not found`, Run the following command

```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

or you can run the executable by `./fairyringclient start` after `go build`
