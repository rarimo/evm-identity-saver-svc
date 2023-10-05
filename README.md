# evm-identity-saver-svc

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Decentralized Oracle service to observe Iden3 state smart contract events and submit them to Rarimo core.

## Configuration
The following configuration .yaml file should be provided to launch your oracle:

```yaml
log:
  disable_sentry: true
  level: debug

## Port to listen incoming requests (used to trigger revote for some operation - rare flow)
listener:
  addr: :8000

evm:
  ## State contract for listening
  contract_addr: ""
  ## Websocket address
  rpc: ""
  ## Start block to listen events. (0 - listen from current). Used to catchup old events. Be careful to use.
  start_from_block:
  ## According to network name in Rarimo core. Example: Polygon
  network_name:
  ## How many blocks should be created after event to fetch it.
  block_window:

broadcaster:
  ## address of broadcaster-svc
  addr: ""
  ## sender account address (rarimo..)
  sender_account: ""

## Rarimo chain connections

core:
  addr: tcp://validator:26657

cosmos:
  addr: validator:9090

## Profiling

profiler:
  enabled: false
  addr: :8080

## Issuer information

state_contract_cfg:
  issuer_id: ['', '']
  disable_filtration: true
```

You will also need some environment variables to run:

```yaml
- name: KV_VIPER_FILE
  value: /config/config.yaml # The path to your config file
```


## Run
To start the service (in vote mode) use the following command:
```shell
evm-identity-saver-svc run state-update-voter
```

To run in full mode:
```shell
evm-identity-saver-svc run state-update-all
```