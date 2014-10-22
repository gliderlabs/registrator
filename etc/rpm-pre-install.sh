#!/bin/bash

getent passwd reg-ator > /dev/null || /usr/sbin/useradd -r reg-ator
exit 0
