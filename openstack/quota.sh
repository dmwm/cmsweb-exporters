#!/bin/bash
# Authors: Muhammad Imran, Ceyhun Uzunoglu
#
# Gets openstack project stats and records, requires keystone_env.sh as input to set required environment variables
#
# !ATTENTION!: Please do not use any echo or print statement
#
# Each function prints one metric. Functions run in parallel to speed up run time and results will be UNORDERED!
# Function names are self expressive
#
usage="Usage: quota.sh <keystone_env.sh>"
if [ $# -ne 1 ] || [ "$1" == "-h" ] || [ "$1" == "-help" ] || [ "$1" == "--help" ]; then
    echo "$usage"
    echo ""
    echo "keystone_env.sh structure:
    #!/bin/sh
    export OS_AUTH_URL=https://keystone.cern.ch/v3
    export OS_PROJECT_DOMAIN_ID=default
    export OS_APPLICATION_CREDENTIAL_SECRET=
    export OS_REGION_NAME=cern
    export OS_APPLICATION_CREDENTIAL_ID=
    export OS_IDENTITY_API_VERSION=3
    export OS_AUTH_TYPE=v3applicationcredential
    export OS_VOLUME_API_VERSION=3
    export OS_APP_NAME=
    "
    echo "Example output:"
    echo "
    shares_used: 0
    shares_size_used_gbytes: 0
    shares_total: 0
    shares_size_total_gbytes: 0
    cpus_total: 0
    ram_total_gbytes: 0
    instances_total: 0
    volumes_total: 0
    volumes_size_total_gbytes: 0
    instances_used: 0
    volumes_used: 0
    volumes_size_used_gbytes: 0
    cpus_used: 0
    ram_used_gbytes: 0
    "
    exit 1
fi

keystone_env="$1"
source "$keystone_env"

# Set common temp file extension
TEMP_FILE_EXTENSION="tmp"

get_quota_show_results() {
    # Save results to temp file to not run same command again
    local quota_show="quota_show.${TEMP_FILE_EXTENSION}"
    openstack quota show >$quota_show

    echo cpus_total: "$(grep core <${quota_show} | awk '{print $4}')"
    # Divide MB to 1024 to get GB
    echo ram_total_gbytes: "$(grep ram <${quota_show} | awk '{print $4/1024}')"
    echo instances_total: "$(grep instances <${quota_show} | awk '{print $4}')"
    echo volumes_total: "$(grep ' volumes ' <${quota_show} | awk '{print $4}')"
    echo volumes_size_total_gbytes: "$(grep ' gigabytes ' <${quota_show} | awk '{print $4}')"
    rm $quota_show
}
get_volume_list_results() {
    # Save results to temp file to not run same command again
    local volume_list="volume_list.${TEMP_FILE_EXTENSION}"
    openstack volume list >$volume_list

    echo volumes_used: "$(tail -n +4 <${volume_list} | grep -c -v +)"
    echo volumes_size_used_gbytes: "$(tail -n +4 <${volume_list} | grep -v + | awk '{ SUM += $8} END { print SUM }')"
    rm $volume_list
}
get_share_quota_show_results() {
    # Save results to temp file to not run same command again
    local share_quota_show="share_quota_show.${TEMP_FILE_EXTENSION}"
    openstack --os-share-api-version 2.57 share quota show >$share_quota_show

    echo shares_total: "$(grep ' shares ' <${share_quota_show} | awk '{print $4}')"
    echo shares_size_total_gbytes: "$(grep ' gigabytes ' <${share_quota_show} | awk '{print $4}')"
    rm $share_quota_show
}
get_share_list_results() {
    # Save results to temp file to not run same command again
    local share_list="share_list.${TEMP_FILE_EXTENSION}"
    openstack --os-share-api-version 2.57 share list >$share_list

    echo shares_used: "$(grep -v + <${share_list} | tail -n +2 | wc -l)"
    echo shares_size_used_gbytes: "$(grep -v + <${share_list} | tail -n +2 | awk '{ SUM += $6} END { print SUM }')"
    rm $share_list
}

get_server_and_flavor_list_results() {
    local instances_list="instances_list.${TEMP_FILE_EXTENSION}"
    local flavors_list="flavors_list.${TEMP_FILE_EXTENSION}"
    local flavor_instance_count="flavor_instance_count.${TEMP_FILE_EXTENSION}"

    # Save to temp file
    openstack server list >$instances_list

    # Get instances_used in the project
    echo instances_used: "$(grep -v + <${instances_list} | grep -c -v Flavor)"

    # Below this point, total used RAM and used CPU in the whole project will be calculated.
    # The logic is:
    #   - Get each flavor's CPU and RAM definition in openstack
    #   - Get all instances and flavor types in the project
    #   - And multiply flavor's CPU and RAM definitions with the project's instance count to get the total numbers

    # the flavor id, ram(MB) and cpu of each flavor type in openstack: r2.xlarge,30000,8
    # Save to temp file
    openstack flavor list -f csv --format value | awk '{print $2","$3","$6}' >$flavors_list

    # instance count of each flavor in the project: 21,m2.2xlarge
    # Save to temp file
    awk '{print $12}' <$instances_list | sort | uniq -c | grep -v Flavor | tail -n +2 | awk '{print $1","$2}' >"$flavor_instance_count"

    # Initialize variables to use in while loop
    cpus_used=0
    ram_used=0

    # Itereate over flavor_instance_count(count, flavor id) lists and get total ram and cpu
    #   by multiplying count with flavor definitions' ram and cpu
    while IFS= read -r line; do
        # line example: 21,m2.2xlarge

        # Number of instance
        number_of_instances=$(echo "$line" | cut -d ',' -f1)

        # Flavor type
        flavor_type=$(echo "$line" | cut -d ',' -f2)

        # Get a flavor type's RAM and CPU definition and multiply with the instance count of the project
        cpus_used=$((cpus_used + $(grep "$flavor_type" <$flavors_list | cut -d ',' -f3) * number_of_instances))
        ram_used=$((ram_used + $(grep "$flavor_type" <$flavors_list | cut -d ',' -f2) * number_of_instances))
    done \
        <"$flavor_instance_count"

    # Divide MB to 1024 to get GB
    echo cpus_used: $cpus_used
    echo ram_used_gbytes: $((ram_used / 1024))

    # Delete temp files
    rm $instances_list
    rm $flavors_list
    rm $flavor_instance_count
}

main() {
    # Delete all temp files and kill all processes ON EXIT
    trap 'rm -rf *.${TEMP_FILE_EXTENSION}; kill 0' SIGINT

    # Run in parallel, stdouts will be unordered
    get_quota_show_results &
    get_volume_list_results &
    get_share_quota_show_results &
    get_share_list_results &
    get_server_and_flavor_list_results &
    # Wait to finish all processes
    wait
}

main
