# Static

Static assets are stored alongside the project in order to be embedded into the main go binary.

This allows JS dependencies to be served locally without internet in the event there is no connection.

Sources 

```shell
# Datastar
wget 'https://cdn.jsdelivr.net/gh/starfederation/datastar@main/bundles/datastar.js'
# Chart JS
wget 'https://cdn.jsdelivr.net/npm/chart.js@4.4.4/dist/chart.umd.min.js'
# Chart JS Date Adaptor
wget 'https://cdn.jsdelivr.net/npm/chartjs-adapter-date-fns/dist/chartjs-adapter-date-fns.bundle.min.js'
# Chart JS Realtime Streaming 
wget 'https://cdn.jsdelivr.net/npm/chartjs-plugin-streaming@2'
```