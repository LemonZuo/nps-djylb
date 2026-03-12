#!/bin/sh

if [ ! -d /conf ]; then
    mkdir -p /conf
fi

if [ ! -f /conf/nps.conf ]; then
    cp /nps.conf.sample /conf/nps.conf
fi

/nps service
