#!/bin/bash
# Author: Valentin Kuznetsov
# process_monitor.sh script starts new process_exporter for given
# pattern and prefix. It falls into infinitive loop with given interval
# and restart process_exporter for our pattern process.

usage="Usage: process_monitor.sh <patter> <prefix> <interval>"
if [ $# -ne 3 ]; then
    echo $usage
    exit 1
fi
if [ "$1" == "-h" ] || [ "$1" == "-help" ] || [ "$1" == "--help" ]; then
    echo $usage
    exit 1
fi

# setup our input parameters
pat=$1
prefix=$2
interval=$3
while :
do
    # find pid of our pattern
    pid=`ps auxw | grep "$pat" | grep -v grep | grep -v process_exporter | awk '{print $2}'`
    if [ -z "$pid" ]; then
        echo "No pattern '$pat' found"
        sleep $interval
        continue
    fi

    # check if process_exporter is running
    out=`ps auxw | grep "process_exporter -pid $pid -prefix $prefix" | grep -v grep`
    if [ -n "$out" ]; then
        echo "Found existing process_exporter: $out"
        sleep $interval
        continue
    fi

    # find if there is existing process_exporter process
    out=`ps auxw | grep "process_exporter -pid [0-9]* -prefix $prefix" | grep -v grep`
    if [ -n "$out" ]; then
        echo "Killing existing process_exporter: $out"
        prevpid=`echo "$out" | awk '{print $2}'`
        kill -9 $prevpid
    fi

    # start new process_exporter process
    echo "Starting: process_exporter -pid=$pid -prefix $prefix"
    nohup process_exporter -pid $pid -prefix $prefix 2>&1 1>& /dev/null < /dev/null &

    # sleep our interval for next iteration
    sleep $interval
done
