'use strict';

const axios = require('axios');
const prometheus = require('prom-client');
const register = prometheus.register;
const Gauge = prometheus.Gauge;

const express = require('express');
const server = express();

const _eval = require('eval');

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

const speed_gauge = new Gauge({
        name: 'gs1200_speed',
        help: 'Port speed in Mbps.',
        labelNames: ['port', 'status', 'loop']
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

function getMetrics() {
    return axios
        .post('http://'+address+'/login.cgi', new URLSearchParams({password: password}))
        .then(() => {
            let data = {};
            return axios.get('http://'+address+'/system_data.js')
                .then(response => {
                    if (response.data.match(/<\/html>/))
                        console.log('Error fetching code; logged in elsewhere?');
                    else {
                        data['system'] = _eval(response.data+'exports.Max_port=Max_port;exports.model_name=model_name;exports.sys_fmw_ver=sys_fmw_ver;exports.sys_MAC=sys_MAC;exports.sys_IP=sys_IP;exports.loop=loop;exports.loop_status=loop_status');
                    }
                })
                .then(() => {
                    axios.get('http://'+address+'/link_data.js')
                    .then(response => {
                        if (response.data.match(/<\/html>/))
                            console.log('Error fetching code; logged in elsewhere?');
                        else {
                            data['link'] = _eval(response.data+'exports.portstatus=portstatus;exports.speed=speed;exports.Stats=Stats;');
                        }
                    })
                    .then(() => {
                        if ('system' in data && 'link' in data) {
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
                                speed_gauge.set({port: port, status: data.link.portstatus[i], loop: data.system.loop_status[i]}, parseInt(data.link.speed[i])*1000*1000);
                                tx_gauge.set({port: port}, data.link.Stats[i][1]+data.link.Stats[i][2]+data.link.Stats[i][3]);
                                rx_gauge.set({port: port}, data.link.Stats[i][6]+data.link.Stats[i][7]+data.link.Stats[i][8]+data.link.Stats[i][10]);
                            }
                        }
                    });
                });
        })
        .then(() => { return axios.get('http://'+address+'/logout.html'); })
        .catch(error => { console.log(error); });
}

process.on('SIGINT', function() {
  process.exit();
});

server.get('/metrics', async (req, res) => {
    await getMetrics();
    res.set('Content-Type', register.contentType);
    res.end(register.metrics());
});

server.listen(port);
