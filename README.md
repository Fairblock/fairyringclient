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

## Setting up the client

### Init the config file & keys directory

This command will create a config directory under your `$HOME` directory: `$HOME/.fairyringclient`.
It will also create the RSA key for interacting with the ShareAPI for you automatically,
you can disable this by adding `--no-rsa-key` flag.

If you would like it to generate a new cosmos private key for you, you can add `--with-cosmos-key` flag

```bash
fairyringclient config init
```

### Showing the config

After initializing the config, you will be able to see the config detail by:
```bash
fairyringclient config show
```

### Updating the config

You can update the config by editing the config file in `$HOME/.fairyringclient/config.yml` or you can use the following command:

```bash
fairyringclient config update
```

You can only update the node config with this command, if you would like to update RSA Keys / Cosmos Keys, please refer to [this section]()

Here are the available flags for updating the config:

```bash
--chain-id string   Update config chain id (default "fairytest-1")
--denom string      Update config denom (default "ufairy")
--grpc-port uint    Update config grpc-port (default 9090)
--ip string         Update config node ip address (default "127.0.0.1")
--port uint         Update config node port (default 26657)
--protocol string   Update config node protocol (default "http")
```

#### Example on updating config

Lets say you would like to update the chain id to `"fairyring"` and the denom to `"fairy"`, you can execute the following command:

```bash
fairyringclient config update --chain-id fairyring --denom fairy
```

### Manging RSA Keys & Cosmos Account Private Keys

#### RSA Keys

##### Adding RSA Keys

You can add RSA Key to the client by using

```bash
fairyringclient keys rsa add [path-to-rsa-private-key]
```

This command will automatically derive the public key, rename the key file and add them to the keys directory `$HOME/.fairyringclient/keys` for you

##### Listing RSA Keys

You can list all the RSA Key in the keys directory by

```bash
fairyringclient keys rsa list
```

#### Cosmos Account Keys

##### Adding Cosmos Account Private Key

You can add a private key to the client by

```bash
fairyringclient keys cosmos add [private-key-in-hex]
```

Or you can add the private key to the `config.yml` manually under the `privatekeys` section:

```bash
privatekeys:
    - private_key_1
    - private_key_2
```

##### Listing all the Cosmos Account Private Key

This command will list all the private keys added to the client config

```bash
fairyringclient keys cosmos list
```

##### Removing Cosmos Account Private Key

You can remove a specific private key with this command

```bash
fairyringclient keys cosmos remove [private_key_index]
```

You can get the private key index by using the `fairyringclient keys cosmos list` command

**Make sure the account you are using already activated and have enough balance for sending transaction and 
you have te same number of RSA keys and the number of cosmos private keys**

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
