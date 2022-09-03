#!/bin/bash


go run ./cmd/erigon --private.api.addr=0.0.0.0:9090 \
      --datadir=/erigon \
      --http=false \
      --metrics --metrics.addr=0.0.0.0 --metrics.port=6060 \
      --chain=goerli

