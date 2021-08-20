#!/bin/sh
set -e

# By default sleep 900 seconds (15 minutes)
[[ ! $STOP_TIMEOUT ]] && STOP_TIMEOUT=900

killProcess() {
  echo "Shutting down registrator" 
  kill -s SIGTERM $$
}

delayExitOnQuitSignal() {
  echo "Daemonset received signal, await for containers to be killed before quitting"

  local duration
  local running_tasks
  # we want to check here if other container are stopped
  # to exit before sleep timeout if possible
  duration=0
  timeout="${STOP_TIMEOUT}"
  while [[ $duration -lt $timeout ]]; do
    echo "got here ...${duration}"
    # checking the docker containers duration except filebeat and ecs-agent
    # exit when no other containers left or exit on timeout
    running_tasks=$(docker container ls --format '{{.Image}}' | grep -v 'registrator\|restrainer\|haproxy\|filebeat\|amazon-ecs-agent\|amazon-ecs-pause')
    echo "running_tasks==>${running_tasks}"
    if [[ -z $running_tasks ]]; then
      killProcess
    else
      sleep 5
      duration=$((duration+5))
      continue
    fi
  done

  killProcess
}

trap delayExitOnQuitSignal SIGQUIT

/bin/registrator -ip=$(ip -o -4 addr list eth0 | head -n1 | awk '{print $4}' | cut -d/ -f1) $@