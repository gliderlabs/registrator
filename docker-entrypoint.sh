#!/bin/sh
set -e

REGISTRATOR_BIND=
if [ -n "$REGISTRATOR_BIND_INTERFACE" ]; then
  REGISTRATOR_BIND_ADDRESS=$(ip -o -4 addr list $REGISTRATOR_BIND_INTERFACE | head -n1 | awk '{print $4}' | cut -d/ -f1)
  if [ -z "$REGISTRATOR_BIND_ADDRESS" ]; then
    echo "Could not find IP for interface '$REGISTRATOR_BIND_INTERFACE', exiting"
    exit 1
  fi

  REGISTRATOR_BIND="-ip $REGISTRATOR_BIND_ADDRESS"
  echo "==> Found address '$REGISTRATOR_BIND_ADDRESS' for interface '$REGISTRATOR_BIND_INTERFACE', setting bind option..."
fi

set -- registrator \
    $REGISTRATOR_BIND \
    "$@"

exec "$@"
