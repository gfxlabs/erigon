#!/bin/bash


go run ./cmd/rpcdaemon  \
      --datadir=/erigon \
      --chain=goerli \
      --ws --ws.compression --http.compression \
      -http.addr=0.0.0.0 --http.vhosts=* --http.corsdomain=* \
      --http.api=eth,erigon,web3,net,trace,debug,txpool \
      --private.api.addr=erigon:9090 \
      --datadir=/home/erigon/.local/share/erigon \
      --rpc.batch.concurrency=8 \
      --db.read.concurrency=512 \
      --metrics --metrics.addr=0.0.0.0 --metrics.port=6064 \
      --pprof --pprof.addr=0.0.0.0 --pprof.port=6000
