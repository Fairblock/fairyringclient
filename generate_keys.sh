#!/bin/bash

die () {
    echo >&2 "$@"
    exit 1
}

[ "$#" -eq 2 ] || die "2 argument required, $# provided, Usage: ./generate_keys {start_at} {end_at}"

echo $1 | grep -E -q '^[0-9]+$' || die "Numeric argument required, $1 provided"
echo $2 | grep -E -q '^[0-9]+$' || die "Numeric argument required, $2 provided"

for i in $(seq $1 $2 $END)
do
  ssh-keygen -t rsa -b 2048 -m PEM -f "./keys/sk$i.pem" -N ""
  ssh-keygen -f "./keys/sk$i.pem" -e -m pem > "./keys/pk$i.pub"
  rm "./keys/sk$i.pem.pub"
done