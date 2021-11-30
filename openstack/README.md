# Openstack Quota Exporter

Prometheus exporter which gets openstack project's quota metrics using openstack CLI

## Prerequisites

- [go](https://go.dev/) Used version is go1.17.3
- `keystone.sh` file which has below format:

```shell
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
```
- Used `bash` version: GNU bash, version 4.2.46
- **scrape_interval** and **scrape_timeout** should be greater than 60 seconds in Prometheus configuration. I.e:
```yaml
- job_name: 'quota-exporter'
  scrape_interval: 120s
  scrape_timeout: 110s
  static_configs:
    - targets: ['quota-exporter.default.svc.cluster.local:18000']
```

## How to run

- Run `quota.sh` and get all metrics in YAML format:

```shell
chmod +x quota.sh && ./quota.sh keystone_env.sh
```

- Run`quota_exporter.go`:

```shell
chmod +x quota.sh quota_exporter.go && \
go mod tidy && \
go build quota_exporter.go && \
./quota_exporter -script ./quota.sh -env /etc/secrets/keystone_env.sh

# Or
./quota_exporter -script /data/quota.sh
                 -env /etc/secrets/keystone_env.sh
                 -namespace "openstack"
                 -address ":18000"
                 -endpoint "/metrics"
```

## Example openstack command outputs in bash functions

- **get_quota_show_results**: `openstack quota show`
```shell
$ openstack quota show # will be like, not exactly

+------------------------+----------------------------------------------+
| Field                  | Value                                         |
+------------------------+-----------------------------------------------+
| gigabytes              | 100000                                        |
| instances              | 100                                           |
| ram                    | 1000000                                       |
| volumes                | 10                                            |
+------------------------+-----------------------------------------------+
```
- **get_volume_list_results**: `openstack volume list`
```shell
$ openstack volume list # will be like, not exactly

+------+-------+-----------+------+------------------------------+
| ID   | Name  | Status    | Size | Attached to                  |
+------+-------+-----------+------+------------------------------+
| xxxx | pvc-1 | in-use    |    5 | Attached to xxxx on /dev/xxx |
| yyyy | pvc-2 | in-use    |  100 | Attached to yyyy on /dev/xxx |
| zzzz | pvc-3 | available |  400 |                              |
| +----+-------+-----------+------+------------------------------+
```
- **get_share_quota_show_results**: `openstack --os-share-api-version 2.57 share quota show`
```shell
$ openstack --os-share-api-version 2.57 share quota show # will be like, not exactly

+-----------+-------+
| Field     | Value |
+------------+------+
| gigabytes | 1000  |
| shares    | 10    |
+-----------+-------+
```
- **get_share_list_results**: `openstack --os-share-api-version 2.57 share list`
```shell
$openstack --os-share-api-version 2.57 share list # will be like, not exactly

+----+---------+------+-------------+-----------+-----------+------------------+------+-------------------+
| ID | Name    | Size | Share Proto | Status    | Is Public | Share Type Name  | Host | Availability Zone |
+---+----------+------+-------------+-----------+-----------+------------------+------+-------------------+
| x1 | x-share |   10 | CEPHFS      | available | False     | X CephFS         |      | N                 |
| x2 | pvc-x   |    6 | CEPHFS      | available | False     | Y CephFS Testing |      | N                 |
| x3 | pvc-y   |    2 | CEPHFS      | available | False     | X CephFS         |      | N                 |
| x4 | pvc-z   |   11 | CEPHFS      | available | False     | X CephFS         |      | N                 |
+----+---------+------+-------------+-----------+-----------+------------------+------+-------------------+
```
- **get_server_and_flavor_list_results**: `openstack server list` and `openstack flavor list`
```shell
$ openstack server list # will be like, not exactly
+------+-------------------+--------+------------+-------+------------+
| ID   | Name              | Status | Networks   | Image | Flavor     |
+------+-------------------+--------+------------+-------+------------+
| aaaa | instance-2-node-0 | ACTIVE | ipv4, ipv6 |       | m2.xlarge  |
| bbbb | instance-1-node-3 | ACTIVE | ipv4, ipv6 |       | m2.xlarge  |
| cccc | instance-1-node-1 | ACTIVE | ipv4, ipv6 |       | m2.xlarge  |
+------+-------------------+--------+------------+-------+------------+


$ openstack flavor list -f csv --format value # not exactly
00001 r2.xlarge 30000 80 0 8 False
00002 m2.large 7500 40 0 4 True
00003 m2.small 1875 10 0 1 True

```
