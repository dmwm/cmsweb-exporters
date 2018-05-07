### cmsweb-exporters
This repository contains collection of prometheus exporters for various
cmsweb data-services. So far we have implementation for das2go and WMCore
exporters. The former is based on gopsutil metrics available in
[das2go code](https://github.com/dmwm/das2go/blob/master/web/handlers.go#L335).
The later is based on metrics available in [wmcore code](https://github.com/dmwm/WMCore/blob/master/src/python/Utils/ProcessStats.py).


The exporters are written in Go language and we can build and call them as
following:
```
# build das2go exporter
go build das2go_exporter.go

# call das2go exporter
das2go_exporter -uri https://host.cern.ch/das/status

# build wmcore exporter
go build wmcore_exporter.go

# call wmcore exporter, replace app with your favorite WMCore data-services
# e.g. dbs or reqmgr2
wmcore_exporter -uri https://host.cern.ch/app/status -namespace app
```

### References
1. [Prometheus setup](https://prometheus.io/docs/introduction/first_steps/)
