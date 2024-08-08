#!/bin/bash -exu

SYMFONY_BIN="./symfony-cli"
PROXY_JSON="$HOME/.symfony5/proxy.json"
CURL="curl --proxy localhost:7080"

which php || /bin/sh -c 'apt update; apt install -y php-fpm'

$SYMFONY_BIN proxy:domain:attach test

$SYMFONY_BIN proxy:backend:attach test --basepath /httpbin --backend https://httpbin.org/

$SYMFONY_BIN proxy:status

cat $PROXY_JSON

# $CURL https://example.wip

$CURL http://example.wip/httpbin/get
$CURL https://example.wip/httpbin/get
$CURL https://example.wip
