#!/bin/bash

## helper script for updating dependencies; too much shell-fu for a Makefile
## expects GOPATH to be set, requires godep

set -e

PKG_PATH="${1}"
export PATH="${GOPATH}/bin:${PATH}"

## update existing deps; exclude built-in packages
go list -f '{{ range .Imports }}{{.}} {{end}}' ./... \
    | tr ' ' '\n' \
    | egrep -v "${PKG_PATH}" \
    | egrep '\.(com|org|net)/' \
    | sort -u \
    | while read x; do
        echo "==> updating $x"
        go get -u $x
        ## might fail if we change the import path of a package; expect the save
        ## to save us
        godep update $x || true
    done

## save new deps pulled in by updates
godep save ./...
