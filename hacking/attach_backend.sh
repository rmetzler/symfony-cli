#!/bin/bash -exu

SYMFONY_BIN=./symfony-cli
PROXY_JSON=~/.symfony5/proxy.json

$SYMFONY_BIN proxy:backend:attach test --basepath /httpbin --backend https://httpbin.org/
$SYMFONY_BIN proxy:status
cat $PROXY_JSON
