#!/bin/sh

/app/tailscaled --tun=userspace-networking --socks5-server=localhost:1055 &
/app/tailscale up --auth-key=${TAILSCALE_AUTHKEY}
echo Tailscale started
export PATH=$PATH:/app
/app/magicmagicdns
