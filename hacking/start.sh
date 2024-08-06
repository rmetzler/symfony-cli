#!/bin/bash -exu

SYMFONY_BIN="./symfony-cli"
PROXY_JSON="$HOME/.symfony5/proxy.json"

echo
echo "delete the binary to avoid confusion when the code does not compile"
rm -f $SYMFONY_BIN

go build -buildvcs=false .

$SYMFONY_BIN server:ca:install
$SYMFONY_BIN server:start &

sleep 5

$SYMFONY_BIN proxy:start --foreground &
$SYMFONY_BIN proxy:status

cat $PROXY_JSON

echo

pgrep --list-full symfony
