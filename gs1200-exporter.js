'use strict';

require('log-timestamp');

const axios = require('axios');
const prometheus = require('prom-client');
const register = prometheus.register;
const Gauge = prometheus.Gauge;

const express = require('express');
const server = express();

const vm = require('vm');

/* Collect default metrics */
prometheus.collectDefaultMetrics({ register });

const address = process.env.GS1200_ADDRESS;
const port = 'GS1200_PORT' in process.env ? process.env.GS1200_PORT : 9707;
const password = process.env.GS1200_PASSWORD;

const num_ports_gauge = new Gauge({
    name: 'gs1200_num_ports',
    help: 'Number of ports. Mainly a placeholder for system information.',
    labelNames: ['model', 'firmware', 'ip', 'mac', 'loop']
});
const num_vlans_gauge = new Gauge({
    name: 'gs1200_num_vlans',
    help: 'Number of configured vlans.',
    labelNames: ['vlans']
})
const speed_gauge = new Gauge({
    name: 'gs1200_speed',
    help: 'Port speed in Mbps.',
    labelNames: ['port', 'status', 'loop', 'pvlan', 'vlans']
});
const tx_gauge = new Gauge({
    name: 'gs1200_packets_tx',
    help: 'Number of packets transmitted.',
    labelNames: ['port']
});
const rx_gauge = new Gauge({
    name: 'gs1200_packets_rx',
    help: 'Number of packets received.',
    labelNames: ['port']
});

async function getData() {
    let scripts = {
        'system_data.js': 'system',
        'link_data.js': 'link',
        'VLAN_1Q_List_data.js': 'vlan'
    };
    let data = {};
    let promise = Promise.resolve();
    promise = promise.then(() => axios
        .post('http://'+address+'/login.cgi', new URLSearchParams({password: password}))
    );
    for (let [script, id] of Object.entries(scripts)) {
        promise = promise.then(axios
            .get('http://'+address+'/'+script)
            .then(response => {
                if (response.data.match(/<\/html>/)) {
                    throw 'Error fetching '+script+'; logged in elsewhere?';
                }
                const context = {};
                vm.createContext(context);
                vm.runInContext(response.data, context);
                data[id] = context;
            })
        );
    }
    return promise
        .then(axios.get('http://'+address+'/logout.html'))
        .then(() => { return data; });
}

async function getMetrics() {
    register.resetMetrics();
    await getData()
    .then(data => {
        num_ports_gauge.set({
            model: data.system.model_name,
            firmware: data.system.sys_fmw_ver,
            mac: data.system.sys_MAC,
            ip: data.system.sys_IP,
            loop: data.system.loop
        }, parseInt(data.system.Max_port));
        for (var i=0; i<parseInt(data.system.Max_port); i++) {
            let port = 'port '+(i+1);
            let status = data.link.portstatus[i];
            let pvlan = 0;
            let vlans = [];
            for (var j=0; j<data.vlan.qvlans.length; j++) {
                if ((data.vlan.qvlans[j][1] >> i) & 1) {
                    if ((data.vlan.qvlans[j][2] >> i) & 1)
                        vlans.push(parseInt(data.vlan.qvlans[j][0])).toString();
                    else
                        pvlan = parseInt(data.vlan.qvlans[j][0]).toString();
                }
            }
            speed_gauge.set({
                port: port,
                status: data.link.portstatus[i],
                loop: data.system.loop_status[i],
                pvlan: pvlan,
                vlans: vlans.join(',')
            }, parseInt(data.link.speed[i])*1000*1000);
            tx_gauge.set({
                port: port
            }, data.link.Stats[i][1]+data.link.Stats[i][2]+data.link.Stats[i][3]);
            rx_gauge.set({
                port: port
            }, data.link.Stats[i][6]+data.link.Stats[i][7]+data.link.Stats[i][8]+data.link.Stats[i][10]);
        }
        num_vlans_gauge.set({
            vlans: data.vlan.qvlans.map(vlan => { return parseInt(vlan[0]).toString(); }).join(',')
        }, data.vlan.qvlans.length);
    })
    .catch(error => {
        console.log(error);
    });
}

process.on('SIGINT', function() {
  process.exit();
});

server.get('/metrics', async (req, res) => {
    console.log(" Metrics request from %s.", req.headers['x-forwarded-for'] || req.ip);
    await getMetrics();
    res.set('Content-Type', register.contentType);
    res.end(register.metrics());
});

console.log("Starting gs1200-exporter.");
server.listen(port);
