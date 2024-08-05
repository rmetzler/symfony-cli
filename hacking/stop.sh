#!/bin/bash -exu

SYMFONY_BIN="./symfony-cli"
PROXY_JSON="$HOME/.symfony5/proxy.json"
PROXY_JSON_DELETE=false

$SYMFONY_BIN proxy:stop
$SYMFONY_BIN proxy:status

$SYMFONY_BIN server:stop


if [[ -f "$PROXY_JSON" ]]
then
    cat $PROXY_JSON
    $PROXY_JSON_DELETE && rm $PROXY_JSON
fi

echo
pgrep symfony
