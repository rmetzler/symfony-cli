#!/bin/bash -exu

SYMFONY_BIN=./symfony-cli
PROXY_JSON=~/.symfony5/proxy.json

$SYMFONY_BIN proxy:stop
$SYMFONY_BIN proxy:status

$SYMFONY_BIN server:stop


if [[ -f "$PROXY_JSON" ]]
then
    cat $PROXY_JSON
    rm $PROXY_JSON
fi

echo
pgrep symfony
