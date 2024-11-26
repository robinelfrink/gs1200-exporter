package internal

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	log "github.com/sirupsen/logrus"
)

const (
	namespace = "gs1200"
)

var (
	num_ports_metric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "num_ports"),
		"Number of ports. Mainly a placeholder for system information.",
		[]string{"model", "firmware", "ip", "mac", "loop"}, nil)
	num_vlans_metric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "num_vlans"),
		"Number of configured vlans.",
		[]string{"vlans"}, nil)
	speed_metric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "speed"),
		"Port speed.",
		[]string{"port", "status", "loop", "pvlan", "vlans", "unit", "duplex"}, nil)
	tx_metric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "packets_tx"),
		"Number of packets transmitted.",
		[]string{"port"}, nil)
	rx_metric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "packets_rx"),
		"Number of packets received.",
		[]string{"port"}, nil)
	power_metric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "power"),
		"Power usage in Watts.",
		[]string{"port"}, nil)
	max_power_metric = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "max_power"),
		"Maximum power available to PoE ports in Watts.",
		[]string{"led"}, nil)
)

type Exporter struct {
	port      string
	collector Collector
}

func GS1200Exporter(collector Collector, port string) *Exporter {
	return &Exporter{
		collector: collector,
		port:      port,
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- num_ports_metric
	ch <- num_vlans_metric
	ch <- speed_metric
	ch <- tx_metric
	ch <- rx_metric
	ch <- power_metric
	ch <- max_power_metric
}

func (e *Exporter) Run() {
	prometheus.MustRegister(e)
	http.Handle("/metrics", promhttp.Handler())
	log.Info("Listening for requests on port ", e.port)
	log.Fatal(http.ListenAndServe(":"+string(e.port), nil))
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	systemData, portData, err := e.collector.Collect()
	if err != nil {
		log.Error("Collect failed")
		return
	}

	ch <- prometheus.MustNewConstMetric(num_ports_metric, prometheus.GaugeValue,
		float64(systemData.Max_port), systemData.model_name, systemData.sys_fmw_ver,
		systemData.sys_IP, systemData.sys_MAC, systemData.loop)

	ch <- prometheus.MustNewConstMetric(num_vlans_metric, prometheus.GaugeValue,
		float64(len(systemData.vlans)), strings.Join(systemData.vlans, ","))

	if strings.HasSuffix(systemData.model_name, "HP v2") {
		ch <- prometheus.MustNewConstMetric(max_power_metric, prometheus.GaugeValue,
			float64(systemData.total_power), strconv.Itoa(systemData.max_led_power))
	}

	for i, port := range *portData {
		ch <- prometheus.MustNewConstMetric(speed_metric, prometheus.GaugeValue,
			float64(port.speed), port.name, port.portstatus, port.loop_status,
			port.pvlan, strings.Join(port.vlans, ","), port.speedUnit, port.duplex)
		ch <- prometheus.MustNewConstMetric(rx_metric, prometheus.GaugeValue,
			port.stats.rx, port.name)
		ch <- prometheus.MustNewConstMetric(tx_metric, prometheus.GaugeValue,
			port.stats.tx, port.name)
		if strings.HasSuffix(systemData.model_name, "HP v2") && i < 4 {
			ch <- prometheus.MustNewConstMetric(power_metric, prometheus.GaugeValue,
				port.stats.port_power, port.name)
		}
	}

}
