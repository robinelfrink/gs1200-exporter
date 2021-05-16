package main

import (
    "flag"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "net/http/cookiejar"
    "net/url"
    "os"
    "strconv"
    "strings"
    "github.com/dop251/goja"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
    namespace = "gs1200"
)

var (
    listenPort = flag.String("port", "9707",
        "Port on which to expose metrics.",)
    gs1200Address = flag.String("address", "192.168.1.3",
        "IP address or hostname of the GS1200")
    gs1200Password = flag.String("password", "********",
        "Password to log on to the GS1200")

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
        "Port speed in Mbps.",
        []string{"port", "status", "loop", "pvlan", "vlans"}, nil)
    tx_metric = prometheus.NewDesc(
        prometheus.BuildFQName(namespace, "", "packets_tx"),
        "Number of packets transmitted.",
        []string{"port"}, nil)
    rx_metric = prometheus.NewDesc(
        prometheus.BuildFQName(namespace, "", "packets_rx"),
        "Number of packets received.",
        []string{"port"}, nil)
)

func main() {
    flag.Parse()
    exporter := GS1200Exporter(
        getEnv("GS1200_ADDRESS", *gs1200Address),
        getEnv("GS1200_PASSWORD", *gs1200Password),
    )
    prometheus.MustRegister(exporter)
    http.Handle("/metrics", promhttp.Handler())
    fmt.Println("Starting gs1200-exporter.")
    log.Fatal(http.ListenAndServe(":"+getEnv("GS1200_PORT", *listenPort), nil))
}

func getEnv(key, fallback string) string {
    if value, ok := os.LookupEnv(key); ok {
        return value
    }
    return fallback
}

type Exporter struct {
    address, password string
}

func GS1200Exporter(address string, password string) *Exporter {
    return &Exporter {
        address: address,
        password: password,
    }
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
    ch <- num_ports_metric
    ch <- num_vlans_metric
    ch <- speed_metric
    ch <- tx_metric
    ch <- rx_metric
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
    jar, err := cookiejar.New(nil)
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    // Log in on the GS1200. If another user is logged on the device
    // will simply send an empty response, making it impossible
    // to do proper error-handling.
    client := &http.Client{Jar: jar}
    _, err = client.PostForm("http://"+e.address+"/login.cgi",
        url.Values{"password": {e.password}})
    if err != nil {
        fmt.Println("Error:", err)
        // Even though logging in failed, try to log out, clearing the
        // session.
        client.Get("http://"+e.address+"/logout.html")
        return
    }

    // Fetch the javascript files containing all the data.
    // See the samples folder for samples.
    js := ""
    for _, src := range []string{"system_data.js", "link_data.js", "VLAN_1Q_List_data.js"} {
        resp, err := client.Get("http://"+e.address+"/"+src)
        if err != nil {
            fmt.Println("Error:", err)
            client.Get("http://"+e.address+"/logout.html")
            return
        }
        defer resp.Body.Close()
        body, err := ioutil.ReadAll(resp.Body)
        js = js+"\n"+string(body)
    }
    // Clear the session.
    client.Get("http://"+e.address+"/logout.html")

    // Run the javascript code so we can access the data.
    vm := goja.New()
    _, err = vm.RunString(js)
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    // Report system info, using the static number of ports as metrics.
    ch <- prometheus.MustNewConstMetric(num_ports_metric, prometheus.GaugeValue,
        vm.Get("Max_port").ToFloat(),
        vm.Get("model_name").String(),
        vm.Get("sys_fmw_ver").String(),
        vm.Get("sys_IP").String(),
        vm.Get("sys_MAC").String(),
        vm.Get("loop").String())

    // Fetch all per-port data.
    portstatus := vm.Get("portstatus").ToObject(vm)
    loop_status := vm.Get("loop_status").ToObject(vm)
    speed := vm.Get("speed").ToObject(vm)
    stats := vm.Get("Stats").ToObject(vm)
    qvlans := vm.Get("qvlans").ToObject(vm)

    // Loop over ports.
    for i := 1; int64(i) <= vm.Get("Max_port").ToInteger(); i++ {
        var pvlan = "0"
        var vlans []string
        // Loop over configured vlans.
        for _, j := range qvlans.Keys() {
            qvlan := qvlans.Get(j).ToObject(vm)
            if (qvlan.Get("1").ToInteger() >> (i-1)) & 1 > 0 {
                // Current vlan is connected to current port.
                if (qvlan.Get("2").ToInteger() >> (i-1)) & 1 > 0 {
                    // Tagged
                    vlans = append(vlans, qvlan.Get("0").String())
                } else {
                    // Untagged
                    pvlan = qvlan.Get("0").String()
                }
            }
        }

        // Port speed seems to always be "[num] Mbps".
        speed, _ := strconv.Atoi(strings.ReplaceAll(speed.Get(strconv.Itoa(i-1)).String(), " Mbps", ""))
        ch <- prometheus.MustNewConstMetric(speed_metric, prometheus.GaugeValue,
            float64(speed),
            "port "+strconv.Itoa(i),
            portstatus.Get(strconv.Itoa(i-1)).String(),
            loop_status.Get(strconv.Itoa(i-1)).String(),
            pvlan,
            strings.Join(vlans, ","))

        // Sent/received traffic has a weird structure. This is what Zyxel's
        // code does:
        //
        //   tx = Stats[k][1]+Stats[k][2]+Stats[k][3];
        //   rx = Stats[k][6]+Stats[k][7]+Stats[k][8]+Stats[k][10];
        //   tx = parseFloat(tx).toLocaleString(); //Divide the numbers with commas
        //   rx = parseFloat(rx).toLocaleString();
        portstats := stats.Get(strconv.Itoa(i-1)).ToObject(vm)
        ch <- prometheus.MustNewConstMetric(tx_metric, prometheus.CounterValue,
            portstats.Get("1").ToFloat() + portstats.Get("2").ToFloat() + portstats.Get("3").ToFloat(),
            "port "+strconv.Itoa(i))
        ch <- prometheus.MustNewConstMetric(rx_metric, prometheus.CounterValue,
            portstats.Get("6").ToFloat() + portstats.Get("7").ToFloat() + portstats.Get("8").ToFloat() + portstats.Get("10").ToFloat(),
            "port "+strconv.Itoa(i))
    }

    // Report number of configured vlans.
    var vlans []string
    for _, j := range qvlans.Keys() {
        vlans = append(vlans, qvlans.Get(j).ToObject(vm).Get("0").String())
    }
    ch <- prometheus.MustNewConstMetric(num_vlans_metric, prometheus.GaugeValue,
        float64(len(vlans)),
        strings.Join(vlans, ","))
}
