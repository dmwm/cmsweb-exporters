#!/bin/bash
# ATTENTION: Please do not use any echo or print statement
#

if [ $# -ne 1 ]; then
    echo "Usage: ./quota.sh \"path to env file for application credentials\" "
    exit 1
fi

source $1

cpus=$(openstack quota show | grep core | awk '{print $4}')
ram=$(openstack quota show | grep ram | awk '{print $4}')
instances=$(openstack quota show | grep instances | awk '{print $4}')

volumes=$(openstack quota show | grep ' volumes ' | awk '{print $4}')
volume_size=$(openstack quota show | grep ' gigabytes ' | awk '{print $4}')

volumes_used=$(openstack volume list | tail -n +4 | grep -c -v +)
volume_size_used=$(openstack volume list | tail -n +4 | grep -v + | awk '{ SUM += $8} END { print SUM }')
instances_used=$(openstack server list | grep -v + | grep -c -v Flavor)

shares=$(openstack --os-share-api-version 2.57 share quota show | grep ' shares ' | awk '{print $4}')
shares_used=$(openstack --os-share-api-version 2.57 share list | grep -v + | tail -n +2 | wc -l)
shares_size=$(openstack --os-share-api-version 2.57 share quota show | grep ' gigabytes ' | awk '{print $4}')
shares_size_used=$(openstack --os-share-api-version 2.57 share list | grep -v + | tail -n +2 | awk '{ SUM += $6} END { print SUM }')

openstack server list | awk '{print $12}' | sort | uniq -c | grep -v Flavor | tail -n +2 | awk '{print $1","$2}' >server.list

cpus_used=0
ram_used=0

input="server.list"
while IFS= read -r line; do

    number_of_servers=$(echo "$line" | cut -d ',' -f1)
    server_flavor=$(echo "$line" | cut -d ',' -f2)

    cpus_used=$((cpus_used + $(openstack flavor list | grep "$server_flavor" | awk '{print $12}') * number_of_servers))
    ram_used=$((ram_used + $(openstack flavor list | grep "$server_flavor" | awk '{print $6}') * number_of_servers))

done \
    <"$input"

rm server.list

ram=$((ram / 1024))
ram_used=$((ram_used / 1024))

JSON_STRING=$(jq -n \
    --arg cpus "$cpus" \
    --arg cpus_used "$cpus_used" \
    --arg ram "$ram" \
    --arg ram_used "$ram_used" \
    --arg instances "$instances" \
    --arg instances_used "$instances_used" \
    --arg volumes "$volumes" \
    --arg volumes_used "$volumes_used" \
    --arg volume_size "$volume_size" \
    --arg volume_size_used "$volume_size_used" \
    --arg shares "$shares" \
    --arg shares_used "$shares_used" \
    --arg shares_size "$shares_size" \
    --arg shares_size_used "$shares_size_used" \
    "{
    total_cpus: $cpus|tonumber,
    cpus_used: $cpus_used|tonumber,
    total_ram: $ram|tonumber,
    ram_used: $ram_used|tonumber,
    total_instances: $instances|tonumber,
    instances_used: $instances_used|tonumber,
    total_volume: $volumes|tonumber,
    volumes_used: $volumes_used|tonumber,
    total_volume_size: $volume_size|tonumber,
    total_volume_size_used: $volume_size_used|tonumber,
    total_shares: $shares|tonumber,
    shares_used: $shares_used|tonumber,
    shares_size: $shares_size|tonumber,
    shares_size_used: $shares_size_used|tonumber,
    }")

echo "$JSON_STRING"
