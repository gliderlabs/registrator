#!/bin/bash

## helper script for updating dependencies; too much shell-fu for a Makefile
## expects GOPATH to be set, requires godep

set -e

export PATH="${GOPATH}/bin:${PATH}"

## update existing deps; exclude built-in packages
go list -f '{{ range .Imports }}{{.}} {{end}}' \
    | tr ' ' '\n' \
    | egrep '\.(com|org|net)/' \
    | while read x; do
        echo "==> updating $x"
        go get -u $x
        godep update $x
    done

## save new deps pulled in by updates
godep save
