#!/bin/bash

SYMFONY_BIN_AMD=symfony-cli_amd64
SYMFONY_BIN_ARM=symfony-cli_arm64

echo
echo "delete the binary to avoid confusion when the code does not compile"
rm -f $SYMFONY_BIN_AMD* $SYMFONY_BIN_ARM*

GOOS=darwin GOARCH=arm64 go build -o=$SYMFONY_BIN_ARM -buildvcs=false .
GOOS=darwin GOARCH=amd64 go build -o=$SYMFONY_BIN_AMD -buildvcs=false .

zip $SYMFONY_BIN_AMD.zip $SYMFONY_BIN_AMD
zip $SYMFONY_BIN_ARM.zip $SYMFONY_BIN_ARM
