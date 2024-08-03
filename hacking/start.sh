#!/bin/bash -exu

SYMFONY_BIN=./symfony-cli
PROXY_JSON=~/.symfony5/proxy.json

go build .

$SYMFONY_BIN proxy:start
$SYMFONY_BIN proxy:status

cat $PROXY_JSON
