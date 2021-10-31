package main

import (
    "flag"
    "fmt"
    "io/ioutil"
    "log"
    "math/rand"
    "net/http"
    "net/http/cookiejar"
    "net/url"
    "os"
    "strconv"
    "strings"
    "github.com/robertkrimen/otto"
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
    vm *otto.Otto
}

func GS1200Exporter(address string, password string) *Exporter {
    return &Exporter {
        address: address,
        password: password,
        vm: otto.New(),
    }
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
    ch <- num_ports_metric
    ch <- num_vlans_metric
    ch <- speed_metric
    ch <- tx_metric
    ch <- rx_metric
}

func (e *Exporter) GetValue(name string) otto.Value {
    value, _ := e.vm.Get(name)
    return value
}

func (e *Exporter) GetFloat(name string) float64 {
    value, _ := e.GetValue(name).ToFloat()
    return value
}

func (e *Exporter) GetString(name string) string {
    value, _ := e.GetValue(name).ToString()
    return value
}

func (e *Exporter) GetInt(name string) int {
    value, _ := e.GetValue(name).ToInteger()
    return int(value)
}

func (e *Exporter) EncryptPassword(password string) string {
    const letters = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
    bytes := []byte(password)
    result := ""
    for i := 0; i <= len(bytes); i++ {
        result = result + string(letters[rand.Intn(len(letters))])
        if i < len(bytes) {
            result = result + string(int(bytes[i]) - len(bytes))
        }
    }
    return result
}

func (e *Exporter) FetchAndParse(client *http.Client, filename string) {
    resp, err := client.Get("http://"+e.address+"/"+filename)
    if err != nil {
        defer resp.Body.Close()
        fmt.Println("Error:", err)
        resp, _ = client.Get("http://"+e.address+"/logout.html")
        defer resp.Body.Close()
        return
    }
    defer resp.Body.Close()
    body, err := ioutil.ReadAll(resp.Body)
    e.vm.Run(string(body))
    return
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
    jar, err := cookiejar.New(nil)
    if err != nil {
        fmt.Println("Error:", err)
        return
    }
    client := &http.Client{Jar: jar}

    e.FetchAndParse(client, "system_data.js")
    Max_port := e.GetInt("Max_port")
    model_name := e.GetString("model_name")
    sys_fmw_ver := e.GetString("sys_fmw_ver")
    sys_IP := e.GetString("sys_IP")
    sys_MAC := e.GetString("sys_MAC")
    loop := e.GetString("loop")
    loop_statuses, _ := e.GetValue("loop_status").Export()

    // Log in on the GS1200. If another user is logged on the device
    // will simply send an empty response, making it impossible
    // to do proper error-handling.
    pass := e.password
    if sys_fmw_ver != "V2.00(ABME.0)C0" {
	pass = e.EncryptPassword(e.password)
    }
    resp, err := client.PostForm("http://"+e.address+"/login.cgi",
        url.Values{"password": {pass}})
    if err != nil {
        defer resp.Body.Close()
        fmt.Println("Error:", err)
        // Even though logging in failed, try to log out, clearing the
        // session.
        resp, _ := client.Get("http://"+e.address+"/logout.html")
        defer resp.Body.Close()
        return
    }

    // Fetch the javascript files containing all the data.
    e.FetchAndParse(client, "link_data.js")
    portstatuses, _ := e.GetValue("portstatus").Export()
    speeds, _ := e.GetValue("speed").Export()
    statses, _ := e.GetValue("Stats").Export()

    e.FetchAndParse(client, "VLAN_1Q_List_data.js")
    qvlans, _ := e.GetValue("qvlans").Export()

    // Clear the session.
    resp, err = client.Get("http://"+e.address+"/logout.html")
    defer resp.Body.Close()

    // Report system info, using the static number of ports as metrics.
    ch <- prometheus.MustNewConstMetric(num_ports_metric, prometheus.GaugeValue,
        float64(Max_port), model_name, sys_fmw_ver, sys_IP, sys_MAC, loop)

    // Loop over ports.
    for i := 1; i <= Max_port; i++ {
        var pvlan = "0"
        var vlans []string
        // Loop over configured vlans.
        for _, qvlan := range qvlans.([][]string) {
            flag1, _ := strconv.ParseInt(strings.Replace(qvlan[1], "0x", "", -1), 16, 64)
            if (flag1 >> (i-1)) & 1 > 0 {
                // Current vlan is connected to current port.
                flag2, _ := strconv.ParseInt(strings.Replace(qvlan[2], "0x", "", -1), 16, 64)
                if (flag2 >> (i-1)) & 1 > 0 {
                    // Tagged
                    vlans = append(vlans, string(qvlan[0]))
                } else {
                    // Untagged
                    pvlan = string(qvlan[0])
                }
            }
        }

        // Port speed seems to always be "[num] Mbps".
        speed, _ := strconv.Atoi(strings.ReplaceAll(speeds.([]string)[i-1], " Mbps", ""))
        ch <- prometheus.MustNewConstMetric(speed_metric, prometheus.GaugeValue,
            float64(speed),
            "port "+strconv.Itoa(i),
            portstatuses.([]string)[i-1],
            loop_statuses.([]string)[i-1],
            pvlan,
            strings.Join(vlans, ","))

        // Sent/received traffic has a weird structure. This is what Zyxel's
        // code does:
        //
        //   tx = Stats[k][1]+Stats[k][2]+Stats[k][3];
        //   rx = Stats[k][6]+Stats[k][7]+Stats[k][8]+Stats[k][10];
        //   tx = parseFloat(tx).toLocaleString(); //Divide the numbers with commas
        //   rx = parseFloat(rx).toLocaleString();
        stats := statses.([][]interface {})[i-1]
        ch <- prometheus.MustNewConstMetric(tx_metric, prometheus.CounterValue,
            float64(stats[1].(int64) + stats[2].(int64) + stats[3].(int64)),
            "port "+strconv.Itoa(i))
        ch <- prometheus.MustNewConstMetric(rx_metric, prometheus.CounterValue,
            float64(stats[6].(int64) + stats[7].(int64) + stats[8].(int64) + stats[10].(int64)),
            "port "+strconv.Itoa(i))
    }

    // Report number of configured vlans.
    var vlans []string
    for _, qvlan := range qvlans.([][]string) {
        vlans = append(vlans, qvlan[0])
    }
    ch <- prometheus.MustNewConstMetric(num_vlans_metric, prometheus.GaugeValue,
        float64(len(vlans)),
        strings.Join(vlans, ","))
}
