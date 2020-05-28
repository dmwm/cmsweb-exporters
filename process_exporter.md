## process_exporter
Process Exporter for Prometheus server. Used for monit-graphana.

### Used main exporter from:
https://github.com/ncabatoff/process-exporter


### Command to execute:
```
process-exporter -web.listen-address ":<port number>" -config.path config.yml
```
where `config.yml` should describe a process to monitor, e.g.
```
<username>:<processBaseName> e.g:

groupname="user:process-exporter"
```
For example, if we want to monitor python process `test.py` our configuration
file will be
```
"<username>:python: <script>":

{groupname="user:python: test.py",state="Running"}
```
