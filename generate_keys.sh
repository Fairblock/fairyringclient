#!/bin/bash

die () {
    echo >&2 "$@"
    exit 1
}

[ "$#" -eq 2 ] || die "2 argument required, $# provided, Usage: ./generate_keys {start_at} {end_at}"

echo $1 | grep -E -q '^[0-9]+$' || die "Numeric argument required, $1 provided"
echo $2 | grep -E -q '^[0-9]+$' || die "Numeric argument required, $2 provided"

# privateKeysArray=()

for i in $(seq $1 $2 $END)
do
  ssh-keygen -t rsa -b 2048 -m PEM -f "./keys/sk$i.pem" -N ""
  ssh-keygen -f "./keys/sk$i.pem" -e -m pem > "./keys/pk$i.pem"
  rm "./keys/sk$i.pem.pub"
  # pKey=$(openssl rand -hex 32)
  # privateKeysArray+=($pKey)
done

# sed -i .old -e "s/VALIDATOR_PRIVATE_KEYS=/VALIDATOR_PRIVATE_KEYS=$(IFS=, ; echo "${privateKeysArray[*]}")/g" .env
