#!/bin/bash

user="reg-ator"

## create user if it doesn't exist
getent passwd ${user} > /dev/null || /usr/sbin/useradd -r ${user}

## add user to docker group
usermod --append --groups docker ${user}

exit 0
