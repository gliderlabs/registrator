#!/bin/bash
#
# registrator        Service registry bridge for Docker
#       
# chkconfig:   2345 95 95
# description: Service registry bridge for Docker
# processname: registrator
# config: /etc/sysconfig/registrator
# pidfile: /var/run/registrator.pid

### BEGIN INIT INFO
# Provides:       registrator
# Required-Start: $network consul docker
# Required-Stop:
# Should-Start:
# Should-Stop:
# Default-Start: 2 3 4 5
# Default-Stop:  0 1 6
# Short-Description: Service registry bridge for Docker
# Description: Service registry bridge for Docker
### END INIT INFO

# source function library
. /etc/rc.d/init.d/functions

prog="registrator"
user="reg-ator"
group="docker"
exec="/usr/bin/$prog"
pidfile="/var/run/$prog.pid"
lockfile="/var/lock/subsys/$prog"
logfile="/var/log/$prog"
conffile="/etc/sysconfig/$prog"

# pull in sysconfig settings
[ -e $conffile ] && . $conffile

## defaults
export GOMAXPROCS=${GOMAXPROCS:-2}
TTL=${TTL:-0}
TTL_REFERSH=${TTL_REFERSH:-0}
DOCKER_HOST=${DOCKER_HOST:-"unix:///var/run/docker.sock"}

prestart() {
    if ! service consul status > /dev/null ; then
        service consul start
    fi

    if ! service docker status > /dev/null ; then
        service docker start
    fi
}


start() {
    [ -x $exec ] || exit 5
    
    [ -f $conffile ] || exit 6

    umask 077

    touch $logfile $pidfile
    chown $user:$group $logfile $pidfile

    prestart
    
    echo -n $"Starting $prog: "
    
    ## holy shell shenanigans, batman!
    ## go can't be properly daemonized.  we need the pid of the spawned process,
    ## which is actually done via runuser thanks to --user.
    ## you can't do "cmd &; action" but you can do "{cmd &}; action".
    ##
    ## registrator will not write to stdout except in the case of a runtime
    ## panic
    daemon \
        --pidfile=$pidfile \
        --user=$user \
        " { $exec -ttl=${TTL} -ttl-refresh=${TTL_REFERSH} ${REGISTRY_URI} > ${logfile} 2>&1 & } ; echo \$! >| $pidfile "
    
    RETVAL=$?
    
    if [ $RETVAL -eq 0 ]; then
        touch $lockfile
    fi
    
    echo    
    return $RETVAL
}

stop() {
    echo -n $"Stopping $prog: "
    
    killproc -p $pidfile $prog
    RETVAL=$?

    if [ $RETVAL -eq 0 ]; then
        rm -f $lockfile $pidfile
    fi

    echo
    return $RETVAL
}

restart() {
    stop
    start
}

force_reload() {
    restart
}

rh_status() {
    status -p "$pidfile" -l $prog $exec
    
    RETVAL=$?
    
    return $RETVAL
}

rh_status_q() {
    rh_status >/dev/null 2>&1
}

case "$1" in
    start)
        rh_status_q && exit 0
        $1
        ;;
    stop)
        rh_status_q || exit 0
        $1
        ;;
    restart)
        $1
        ;;
    force-reload)
        force_reload
        ;;
    status)
        rh_status
        ;;
    condrestart|try-restart)
        rh_status_q || exit 0
        restart
        ;;
    *)
        echo $"Usage: $0 {start|stop|status|restart|condrestart|try-restart|force-reload}"
        exit 2
esac

exit $?
